// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package tiering

import (
	"context"
	"sync"
	"time"

	"github.com/openartifact/openartifact/pkg/logger"
)

const (
	// AccessResolution is how fresh a recorded access must be before a newer
	// one is written: repeated downloads of the same blob within the window cost
	// no database write. Policies measure idleness in days, so an hour of
	// precision is ample.
	AccessResolution = time.Hour

	// maxPendingAccess bounds the in-memory set of blobs awaiting a flush; an
	// access that does not fit is dropped (the next download records it).
	maxPendingAccess = 50000

	accessFlushTimeout = 30 * time.Second
)

// accessToucher is the database write the tracker flushes with.
type accessToucher interface {
	TouchBlobAccess(ctx context.Context, accessed map[string]time.Time, resolution time.Duration) (int64, error)
}

// AccessTracker coalesces blob download times in memory and writes them to
// blobs.last_accessed_at in one batched statement per flush interval, so a
// download never costs a database write of its own.
type AccessTracker struct {
	db       accessToucher
	log      *logger.Logger
	interval time.Duration

	mu      sync.Mutex
	pending map[string]time.Time

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	started  bool
}

// NewAccessTracker creates a tracker that flushes every interval once Started.
func NewAccessTracker(db accessToucher, interval time.Duration, log *logger.Logger) *AccessTracker {
	return &AccessTracker{
		db: db, log: log, interval: interval,
		pending: make(map[string]time.Time),
		stop:    make(chan struct{}), done: make(chan struct{}),
	}
}

// Touch records a download of the blob at the current time.
func (t *AccessTracker) Touch(sha string) {
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, known := t.pending[sha]; !known && len(t.pending) >= maxPendingAccess {
		return
	}

	t.pending[sha] = now
}

// Pending returns how many blobs await a flush.
func (t *AccessTracker) Pending() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.pending)
}

// Flush writes the pending accesses. On failure they are kept for the next
// flush (never overwriting a newer access recorded meanwhile).
func (t *AccessTracker) Flush(ctx context.Context) error {
	t.mu.Lock()
	batch := t.pending
	t.pending = make(map[string]time.Time)
	t.mu.Unlock()

	if len(batch) == 0 {
		return nil
	}

	if _, err := t.db.TouchBlobAccess(ctx, batch, AccessResolution); err != nil {
		t.mu.Lock()
		for sha, at := range batch {
			if cur, ok := t.pending[sha]; !ok && len(t.pending) < maxPendingAccess || ok && cur.Before(at) {
				t.pending[sha] = at
			}
		}
		t.mu.Unlock()

		return err
	}

	return nil
}

// Start flushes every interval until Stop. A non-positive interval means no
// periodic flush (Flush can still be called).
func (t *AccessTracker) Start() {
	if t.interval <= 0 {
		close(t.done)

		return
	}

	t.started = true

	go func() {
		defer close(t.done)

		tick := time.NewTicker(t.interval)
		defer tick.Stop()

		for {
			select {
			case <-tick.C:
				t.flushQuiet()
			case <-t.stop:
				return
			}
		}
	}()
}

func (t *AccessTracker) flushQuiet() {
	ctx, cancel := context.WithTimeout(context.Background(), accessFlushTimeout)
	defer cancel()

	if err := t.Flush(ctx); err != nil && t.log != nil {
		t.log.Warnw("blob access flush failed", "error", err)
	}
}

// Stop ends the periodic flush and writes what is still pending. Idempotent.
func (t *AccessTracker) Stop() {
	t.stopOnce.Do(func() { close(t.stop) })

	if t.started {
		<-t.done
	}

	t.flushQuiet()
}
