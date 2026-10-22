// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import "time"

// StorageTier is a named placement target. BackendID "" is the primary store
// the server is configured with; every other tier maps to a storage backend.
type StorageTier struct {
	ID            string
	Name          string
	BackendID     string
	BackendName   string
	BackendKind   string
	BackendStatus string
	Rank          int
	PromoteOnRead bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// StoragePolicy is one ordered lifecycle rule: the first enabled policy whose
// conditions match a blob names the tier the blob belongs on. Unset (nil / "" /
// empty) conditions match everything.
type StoragePolicy struct {
	ID             string
	Name           string
	Enabled        bool
	Position       int
	TargetTierID   string
	FromTierID     string // "" = any tier
	MinAgeSeconds  *int64
	MinIdleSeconds *int64
	MinSizeBytes   *int64
	MaxSizeBytes   *int64
	RepoGlob       string
	Formats        []string
	PathGlob       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Storage tier move states (see storage_tier_moves).
const (
	TierMoveCopying = "copying"
	TierMoveFlipped = "flipped"
)

// StorageTierMove is a row of the mover's crash-recovery journal. Backend ids
// are "" for the primary store.
type StorageTierMove struct {
	SHA256        string
	FromBackendID string
	ToBackendID   string
	State         string
	Reason        string
	SizeBytes     int64
	Attempts      int
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TierUsage is the blob count and byte total placed on one backend ("" =
// primary store).
type TierUsage struct {
	BackendID string
	Blobs     int64
	Bytes     int64
}

// BlobFacts are the per-blob facts a lifecycle policy is evaluated against.
type BlobFacts struct {
	SHA256         string
	SizeBytes      int64
	CreatedAt      time.Time
	LastAccessedAt *time.Time
	BackendID      string // "" = primary store
	TierChangedAt  *time.Time
}

// BlobRef is one repository entry referencing a blob.
type BlobRef struct {
	Repo   string
	Format string
	Path   string
}

// TierScanClause is the SQL-expressible part of one policy: a blob is a
// candidate for it when every set bound holds and it is not already on
// TargetBackendID. The exact policy semantics (repository, format and path
// conditions, first match wins) are evaluated in Go on the returned page.
type TierScanClause struct {
	TargetBackendID string
	// FromBackend restricts the source placement when HasFrom is set.
	HasFrom       bool
	FromBackendID string
	CreatedBefore *time.Time // created_at <= value
	IdleBefore    *time.Time // COALESCE(last_accessed_at, created_at) <= value
	MinSizeBytes  *int64
	MaxSizeBytes  *int64
}

// FlipOutcome is the result of a conditional placement flip.
type FlipOutcome int

// Flip outcomes.
const (
	// FlipDone: the placement now points at the target.
	FlipDone FlipOutcome = iota
	// FlipBlobGone: no blobs row exists any more (deleted meanwhile).
	FlipBlobGone
	// FlipPlacementChanged: the placement is not the expected source.
	FlipPlacementChanged
)
