// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package tiering

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// fakeClockThrottle returns a throttle whose clock never advances and whose
// sleeps are recorded instead of taken.
func fakeClockThrottle(rate int64) (*Throttle, *[]time.Duration) {
	th := NewThrottle(rate)
	now := time.Unix(1_000_000, 0)
	slept := &[]time.Duration{}

	th.nowFn = func() time.Time { return now }
	th.sleep = func(_ context.Context, d time.Duration) error {
		*slept = append(*slept, d)

		return nil
	}

	return th, slept
}

func TestThrottleUnlimitedNeverWaits(t *testing.T) {
	t.Parallel()

	for _, th := range []*Throttle{nil, NewThrottle(0), NewThrottle(-5)} {
		start := time.Now()

		if err := th.Wait(context.Background(), 1<<30); err != nil {
			t.Fatalf("Wait = %v", err)
		}

		if time.Since(start) > 500*time.Millisecond {
			t.Errorf("unlimited throttle waited %v", time.Since(start))
		}
	}
}

// Successive reservations queue up behind each other: that is what makes the
// cap hold for all concurrent copies together.
func TestThrottleReservationsQueue(t *testing.T) {
	t.Parallel()

	th, slept := fakeClockThrottle(1000)

	for range 3 {
		if err := th.Wait(context.Background(), 1000); err != nil {
			t.Fatal(err)
		}
	}

	// 1000 B at 1000 B/s costs one second each: no wait, then 1s, then 2s.
	if len(*slept) != 2 || (*slept)[0] != time.Second || (*slept)[1] != 2*time.Second {
		t.Errorf("sleeps = %v, want [1s 2s]", *slept)
	}
}

func TestThrottleReaderPacesAndPreservesData(t *testing.T) {
	t.Parallel()

	th, slept := fakeClockThrottle(throttleChunk) // one chunk of data per second
	payload := bytes.Repeat([]byte("a"), 2*throttleChunk)

	got, err := io.ReadAll(th.Reader(context.Background(), bytes.NewReader(payload)))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("data altered or read failed: %v", err)
	}

	// The last read queues behind everything before it: all but its own bytes
	// of two seconds' worth of data.
	last := (*slept)[len(*slept)-1]
	if last < time.Second || last > 2*time.Second {
		t.Errorf("last wait = %v, want between 1s and 2s (sleeps %v)", last, *slept)
	}
}

func TestThrottleWaitHonoursCancellation(t *testing.T) {
	t.Parallel()

	th := NewThrottle(1) // one byte per second: any real wait is long
	th.nowFn = func() time.Time { return time.Unix(1_000_000, 0) }

	ctx, cancel := context.WithCancel(context.Background())

	if err := th.Wait(ctx, 10); err != nil { // the first reservation is free
		t.Fatal(err)
	}

	cancel()

	if err := th.Wait(ctx, 10); !errors.Is(err, context.Canceled) {
		t.Errorf("Wait = %v, want context.Canceled", err)
	}
}
