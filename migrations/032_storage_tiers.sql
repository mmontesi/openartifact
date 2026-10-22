-- SPDX-FileCopyrightText: The OpenArtifact Authors
--
-- SPDX-License-Identifier: AGPL-3.0-or-later

-- Storage tiers and lifecycle policies (docs/operations/storage-tiering.md).
--
-- A tier is a named placement target. The tier with backend_id NULL is the
-- primary store the server is configured with (STORAGE_*), seeded as "hot";
-- every other tier maps to a storage_backends row. blobs.storage_backend_id is
-- the authoritative placement of a blob (NULL = primary store); the mover
-- changes it only in a single transaction (FlipBlobPlacement) after the bytes
-- are verified on the target.
--
-- Only metadata-only changes on existing tables here (nullable ADD COLUMN);
-- the supporting index is built concurrently in 032_storage_tiers_indexes.no_tx.sql.

SET LOCAL lock_timeout = '10s';

-- Coalesced download time (flushed in batches, never one write per download)
-- and the time of the last placement flip: a blob read since it arrived on its
-- tier is a promote-on-read candidate.
ALTER TABLE blobs ADD COLUMN last_accessed_at timestamp with time zone;
ALTER TABLE blobs ADD COLUMN tier_changed_at timestamp with time zone;

COMMENT ON COLUMN blobs.storage_backend_id IS 'Authoritative placement: the storage backend holding the blob (NULL = the primary store the server is configured with). Changed only by the tier mover, in one transaction.';
COMMENT ON COLUMN blobs.last_accessed_at IS 'Last client download, coalesced (resolution storage tiering access resolution); NULL = never read since tracking started.';
COMMENT ON COLUMN blobs.tier_changed_at IS 'When the mover last flipped the placement; NULL = never moved.';

CREATE TABLE storage_tiers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(64) NOT NULL,
    backend_id uuid,
    rank integer DEFAULT 0 NOT NULL,
    promote_on_read boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_tiers_pkey PRIMARY KEY (id),
    CONSTRAINT storage_tiers_name_key UNIQUE (name),
    CONSTRAINT storage_tiers_backend_id_fkey FOREIGN KEY (backend_id) REFERENCES storage_backends(id) ON DELETE RESTRICT,
    -- the primary tier is where reads are promoted to, never from
    CONSTRAINT chk_storage_tier_promote CHECK (backend_id IS NOT NULL OR NOT promote_on_read)
);

-- A backend serves at most one tier; exactly one tier is the primary store.
CREATE UNIQUE INDEX idx_storage_tiers_backend ON storage_tiers (backend_id) WHERE (backend_id IS NOT NULL);
CREATE UNIQUE INDEX idx_storage_tiers_primary ON storage_tiers ((true)) WHERE (backend_id IS NULL);

CREATE TRIGGER update_storage_tiers_updated_at
    BEFORE UPDATE ON storage_tiers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE storage_tiers IS 'Named storage tiers. backend_id NULL is the primary (hot) store from the server configuration.';

INSERT INTO storage_tiers (name, backend_id, rank) VALUES ('hot', NULL, 0);

CREATE TABLE storage_policies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(128) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    -- evaluation order, lowest first; the first matching policy decides
    position integer DEFAULT 0 NOT NULL,
    target_tier_id uuid NOT NULL,
    -- only blobs currently on this tier match (NULL = any tier)
    from_tier_id uuid,
    -- age since the blob was first stored (blobs.created_at)
    min_age_seconds bigint,
    -- time since the last download (blobs.last_accessed_at, else created_at)
    min_idle_seconds bigint,
    min_size_bytes bigint,
    max_size_bytes bigint,
    -- the following three match the repositories referencing the blob (any reference)
    repo_glob text,
    formats text[] DEFAULT '{}' NOT NULL,
    path_glob text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_policies_pkey PRIMARY KEY (id),
    CONSTRAINT storage_policies_name_key UNIQUE (name),
    CONSTRAINT storage_policies_target_tier_id_fkey FOREIGN KEY (target_tier_id) REFERENCES storage_tiers(id) ON DELETE RESTRICT,
    CONSTRAINT storage_policies_from_tier_id_fkey FOREIGN KEY (from_tier_id) REFERENCES storage_tiers(id) ON DELETE RESTRICT,
    CONSTRAINT chk_storage_policy_bounds CHECK (
        (min_age_seconds IS NULL OR min_age_seconds >= 0)
        AND (min_idle_seconds IS NULL OR min_idle_seconds >= 0)
        AND (min_size_bytes IS NULL OR min_size_bytes >= 0)
        AND (max_size_bytes IS NULL OR max_size_bytes >= 0)
        AND (min_size_bytes IS NULL OR max_size_bytes IS NULL OR min_size_bytes <= max_size_bytes)
    )
);

CREATE TRIGGER update_storage_policies_updated_at
    BEFORE UPDATE ON storage_policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE storage_policies IS 'Ordered lifecycle policies: the first enabled policy whose conditions match a blob names the tier it belongs on.';

-- One row per blob move in flight or awaiting cleanup. state=copying: bytes are
-- being copied/verified on the target, the source stays authoritative.
-- state=flipped: the placement already points at the target; the old copy on
-- the source is still to be deleted. The mover resumes both states after a crash.
-- No foreign key to blobs: a blob deleted meanwhile still needs its stray copy removed.
CREATE TABLE storage_tier_moves (
    sha256 character varying(64) NOT NULL,
    from_backend_id uuid,
    to_backend_id uuid,
    state character varying(16) NOT NULL,
    reason text DEFAULT '' NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_tier_moves_pkey PRIMARY KEY (sha256),
    CONSTRAINT storage_tier_moves_from_fkey FOREIGN KEY (from_backend_id) REFERENCES storage_backends(id) ON DELETE RESTRICT,
    CONSTRAINT storage_tier_moves_to_fkey FOREIGN KEY (to_backend_id) REFERENCES storage_backends(id) ON DELETE RESTRICT,
    CONSTRAINT chk_storage_tier_move_state CHECK (state IN ('copying', 'flipped')),
    CONSTRAINT chk_storage_tier_move_distinct CHECK (from_backend_id IS DISTINCT FROM to_backend_id)
);

COMMENT ON TABLE storage_tier_moves IS 'Crash-recovery journal of the storage tier mover: one row per blob move that has started and not yet finished cleanup.';
