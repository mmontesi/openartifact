// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openartifact/openartifact/internal/tiering"
	"github.com/openartifact/openartifact/pkg/logger"
)

type stubPasses struct {
	calls atomic.Int32
	stats tiering.PassStats
	err   error
	// block, when set, makes a pass wait for it (or the context).
	block chan struct{}
}

func (s *stubPasses) RunPass(ctx context.Context) (tiering.PassStats, error) {
	s.calls.Add(1)

	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
		}
	}

	return s.stats, s.err
}

func tierWorkerForTest(t *testing.T, p TierPassRunner, interval time.Duration, enabled bool) *TierMoverWorker {
	t.Helper()

	log, err := logger.New("error", "console", "stderr")
	if err != nil {
		t.Fatal(err)
	}

	return NewTierMoverWorker(p, nil, log, interval, enabled)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(time.Millisecond)
	}
}

func TestTierMoverRunOnceHoldsAndReleasesLock(t *testing.T) {
	t.Parallel()

	p := &stubPasses{stats: tiering.PassStats{Moved: 3}}
	w := tierWorkerForTest(t, p, 0, true)

	var held, released atomic.Bool

	w.lock = func(context.Context) (func(), bool, error) {
		held.Store(true)

		return func() { released.Store(true) }, true, nil
	}

	st, busy, err := w.RunOnce(context.Background())
	if err != nil || busy || st.Moved != 3 {
		t.Fatalf("RunOnce = %+v, busy=%v, %v", st, busy, err)
	}

	if !held.Load() || !released.Load() {
		t.Errorf("lock held=%v released=%v", held.Load(), released.Load())
	}

	if last := w.Status().LastPass; last == nil || last.Moved != 3 {
		t.Errorf("status last pass = %+v", last)
	}

	if w.Status().Running {
		t.Error("worker still reports running after the pass")
	}
}

func TestTierMoverSkipsPassWhenAnotherReplicaHoldsLock(t *testing.T) {
	t.Parallel()

	p := &stubPasses{}
	w := tierWorkerForTest(t, p, 0, true)
	w.lock = func(context.Context) (func(), bool, error) { return func() {}, false, nil }

	if _, busy, err := w.RunOnce(context.Background()); err != nil || !busy {
		t.Fatalf("RunOnce = busy %v, %v; want busy", busy, err)
	}

	if p.calls.Load() != 0 {
		t.Error("a pass ran without the cluster lock")
	}

	w.lock = func(context.Context) (func(), bool, error) { return func() {}, false, errors.New("db down") }

	if _, _, err := w.RunOnce(context.Background()); err == nil {
		t.Error("lock error was swallowed")
	}
}

func TestTierMoverTriggerRunsAPass(t *testing.T) {
	t.Parallel()

	p := &stubPasses{}
	w := tierWorkerForTest(t, p, 0, true) // no schedule: only triggers run passes
	w.Start()

	defer w.Stop()

	if !w.Trigger() {
		t.Fatal("Trigger refused on an enabled worker")
	}

	waitFor(t, "the triggered pass", func() bool { return p.calls.Load() == 1 })
}

func TestTierMoverDisabledNeverRuns(t *testing.T) {
	t.Parallel()

	p := &stubPasses{}
	w := tierWorkerForTest(t, p, time.Millisecond, false)
	w.Start()

	if w.Trigger() || w.Status().Enabled {
		t.Error("a disabled worker accepted a trigger")
	}

	w.Stop()

	if p.calls.Load() != 0 {
		t.Error("a disabled worker ran a pass")
	}
}

func TestTierMoverSchedulesAndStopCancelsInFlightPass(t *testing.T) {
	t.Parallel()

	p := &stubPasses{block: make(chan struct{})}
	w := tierWorkerForTest(t, p, time.Millisecond, true)
	w.Start()

	waitFor(t, "a scheduled pass to start", func() bool { return w.Status().Running })

	done := make(chan struct{})

	go func() { w.Stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not cancel the in-flight pass")
	}

	w.Stop() // idempotent
}

func TestTierMoverStopWithoutStart(t *testing.T) {
	t.Parallel()

	tierWorkerForTest(t, &stubPasses{}, time.Second, true).Stop()
}
