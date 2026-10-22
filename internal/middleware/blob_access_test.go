// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openartifact/openartifact/internal/storage"
)

func TestTrackBlobAccess(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/repos/maven/releases/a/b.jar", true},
		{http.MethodHead, "/repos/npm/left-pad", true},
		{http.MethodGet, "/v2/app/blobs/sha256:abc", true},
		{http.MethodPut, "/repos/maven/releases/a/b.jar", false},
		{http.MethodPost, "/v2/app/blobs/uploads/", false},
		{http.MethodGet, "/api/v1/admin/storage", false},
		{http.MethodGet, "/repositories/x", false},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()

			var got bool

			h := TrackBlobAccess(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = storage.AccessTracked(r.Context())
			}))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil))

			if got != tc.want {
				t.Errorf("tracked = %v, want %v", got, tc.want)
			}
		})
	}
}
