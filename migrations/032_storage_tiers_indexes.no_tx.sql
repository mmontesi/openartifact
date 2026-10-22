-- SPDX-FileCopyrightText: The OpenArtifact Authors
--
-- SPDX-License-Identifier: AGPL-3.0-or-later

-- Promote-on-read candidates: blobs on a non-primary backend that were read
-- (see 032_storage_tiers.sql). Built without blocking writes (*.no_tx.sql: one
-- statement at a time). Re-runnable: an interrupted CONCURRENTLY build leaves
-- an INVALID index that IF NOT EXISTS would skip for good, so it is dropped first.
DO $$
DECLARE
    idx regclass;
BEGIN
    SET LOCAL lock_timeout = '10s';

    FOR idx IN
        SELECT i.indexrelid::regclass
        FROM pg_index i
        JOIN pg_class c ON c.oid = i.indexrelid
        WHERE NOT i.indisvalid
          AND c.relnamespace = to_regnamespace(current_schema())
          AND c.relname = 'idx_blobs_tier_reads'
    LOOP
        EXECUTE format('DROP INDEX %s', idx);
    END LOOP;
END
$$;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_blobs_tier_reads
    ON blobs (storage_backend_id, last_accessed_at)
    WHERE storage_backend_id IS NOT NULL AND last_accessed_at IS NOT NULL;
