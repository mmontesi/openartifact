// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http"
	"strings"

	"github.com/openartifact/openartifact/internal/storage"
)

// TrackBlobAccess marks client downloads (GET and HEAD of repository content:
// the /repos/ format routes and the /v2/ registry) so that the tiered blob
// store records the blobs they read as accessed. Reads by background workers
// and admin endpoints carry no mark and never count as a download, which keeps
// "not downloaded for N days" policies and promote-on-read honest.
func TrackBlobAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			(strings.HasPrefix(r.URL.Path, "/repos/") || strings.HasPrefix(r.URL.Path, "/v2/")) {
			r = r.WithContext(storage.WithAccessTracking(r.Context()))
		}

		next.ServeHTTP(w, r)
	})
}
