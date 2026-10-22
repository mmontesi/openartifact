// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package responses

import "time"

// StorageTierResponse is a storage tier with its current placement.
type StorageTierResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Primary is true for the tier backed by the server's configured storage.
	Primary bool `json:"primary"`
	// Backend is the storage backend (by name) the tier maps to; null for the primary tier.
	Backend       *string `json:"backend"`
	BackendKind   string  `json:"backend_kind"`
	BackendStatus string  `json:"backend_status"`
	Rank          int     `json:"rank"`
	PromoteOnRead bool    `json:"promote_on_read"`
	// Available is false when this server could not open the tier's backend.
	Available  bool      `json:"available"`
	BlobCount  int64     `json:"blob_count"`
	TotalBytes int64     `json:"total_bytes"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// StoragePolicyResponse is one lifecycle policy.
type StoragePolicyResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Enabled        bool      `json:"enabled"`
	Position       int       `json:"position"`
	TargetTier     string    `json:"target_tier"`
	FromTier       *string   `json:"from_tier"`
	MinAgeSeconds  *int64    `json:"min_age_seconds"`
	MinIdleSeconds *int64    `json:"min_idle_seconds"`
	MinSizeBytes   *int64    `json:"min_size_bytes"`
	MaxSizeBytes   *int64    `json:"max_size_bytes"`
	RepoGlob       *string   `json:"repo_glob"`
	Formats        []string  `json:"formats"`
	PathGlob       *string   `json:"path_glob"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TierRouteResponse totals the moves between one pair of tiers.
type TierRouteResponse struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int64  `json:"count"`
	Bytes int64  `json:"bytes"`
}

// TierPolicyTotalResponse totals the moves one policy causes.
type TierPolicyTotalResponse struct {
	Policy string `json:"policy"`
	Count  int64  `json:"count"`
	Bytes  int64  `json:"bytes"`
}

// TierMoveSampleResponse is one blob a dry run would move.
type TierMoveSampleResponse struct {
	SHA256         string     `json:"sha256"`
	SizeBytes      int64      `json:"size_bytes"`
	From           string     `json:"from"`
	To             string     `json:"to"`
	Policy         string     `json:"policy"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at"`
}

// TierMoveTotalsResponse is a count and byte total.
type TierMoveTotalsResponse struct {
	Count int64 `json:"count"`
	Bytes int64 `json:"bytes"`
}

// TierDryRunResponse is the result of simulating policies without moving anything.
type TierDryRunResponse struct {
	Scanned   int                       `json:"scanned"`
	Truncated bool                      `json:"truncated"`
	WouldMove TierMoveTotalsResponse    `json:"would_move"`
	Routes    []TierRouteResponse       `json:"routes"`
	Policies  []TierPolicyTotalResponse `json:"policies"`
	Sample    []TierMoveSampleResponse  `json:"sample"`
}

// TierPassResponse summarizes one mover pass.
type TierPassResponse struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Recovered  int       `json:"recovered"`
	Promoted   int       `json:"promoted"`
	Moved      int       `json:"moved"`
	Skipped    int       `json:"skipped"`
	Failed     int       `json:"failed"`
	Bytes      int64     `json:"bytes"`
	Scanned    int       `json:"scanned"`
	Aborted    bool      `json:"aborted"`
}

// TierPendingMoveResponse is a journaled move that has not completed.
type TierPendingMoveResponse struct {
	SHA256    string    `json:"sha256"`
	SizeBytes int64     `json:"size_bytes"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	State     string    `json:"state"`
	Reason    string    `json:"reason"`
	Attempts  int       `json:"attempts"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TierMoverStatusResponse is the state of the background mover.
type TierMoverStatusResponse struct {
	Enabled         bool                      `json:"enabled"`
	Running         bool                      `json:"running"`
	IntervalSeconds int64                     `json:"interval_seconds"`
	LastPass        *TierPassResponse         `json:"last_pass"`
	PendingTotal    int64                     `json:"pending_total"`
	Pending         []TierPendingMoveResponse `json:"pending"`
}
