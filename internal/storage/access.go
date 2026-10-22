// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import "context"

type accessTrackedKey struct{}

// WithAccessTracking marks ctx as serving a client download: a tiered blob
// store records reads made with it as the blob's last access (used by tier
// policies and promote-on-read). Background reads (metadata regeneration,
// scans, the tier mover itself) do not carry the mark and so never count as a
// download.
func WithAccessTracking(ctx context.Context) context.Context {
	return context.WithValue(ctx, accessTrackedKey{}, true)
}

// AccessTracked reports whether ctx was marked by WithAccessTracking.
func AccessTracked(ctx context.Context) bool {
	v, _ := ctx.Value(accessTrackedKey{}).(bool)

	return v
}
