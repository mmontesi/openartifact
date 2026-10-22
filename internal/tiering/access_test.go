// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package tiering

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"
)

type failingToucher struct {
	fail    bool
	flushed map[string]time.Time
}

func (f *failingToucher) TouchBlobAccess(_ context.Context, accessed map[string]time.Time, _ time.Duration) (int64, error) {
	if f.fail {
		return 0, errors.New("database down")
	}

	maps.Copy(f.flushed, accessed)

	return int64(len(accessed)), nil
}

func TestAccessTrackerCoalescesAndKeepsBatchOnFailure(t *testing.T) {
	t.Parallel()

	db := &failingToucher{fail: true, flushed: map[string]time.Time{}}
	tr := NewAccessTracker(db, 0, nil)

	for range 100 {
		tr.Touch("aa")
	}

	tr.Touch("bb")

	if tr.Pending() != 2 {
		t.Fatalf("pending = %d, want 2 (repeat downloads coalesce)", tr.Pending())
	}

	if err := tr.Flush(context.Background()); err == nil {
		t.Fatal("Flush succeeded against a failing database")
	}

	if tr.Pending() != 2 {
		t.Fatalf("batch lost after a failed flush: pending = %d", tr.Pending())
	}

	db.fail = false

	if err := tr.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(db.flushed) != 2 || tr.Pending() != 0 {
		t.Errorf("flushed %v pending %d", db.flushed, tr.Pending())
	}
}

func TestAccessTrackerFlushesPeriodicallyAndOnStop(t *testing.T) {
	t.Parallel()

	db := &failingToucher{flushed: map[string]time.Time{}}
	tr := NewAccessTracker(db, time.Hour, nil)
	tr.Start()
	tr.Touch("cc")
	tr.Stop() // writes what is pending

	if _, ok := db.flushed["cc"]; !ok {
		t.Errorf("Stop did not flush pending accesses: %v", db.flushed)
	}

	tr.Stop() // idempotent
}
