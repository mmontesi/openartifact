// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package tiering

import (
	"context"
	"sort"
	"time"
)

// MaxPlanSample caps the sample of blobs a dry run returns.
const MaxPlanSample = 200

// PlanRoute totals the moves between one pair of tiers.
type PlanRoute struct {
	From  string
	To    string
	Count int64
	Bytes int64
}

// PlanPolicy totals the moves one policy would cause.
type PlanPolicy struct {
	Policy string
	Count  int64
	Bytes  int64
}

// PlanSample is one blob a dry run would move.
type PlanSample struct {
	SHA256         string
	SizeBytes      int64
	From           string
	To             string
	Policy         string
	CreatedAt      time.Time
	LastAccessedAt *time.Time
}

// Plan is the result of a dry run.
type Plan struct {
	// Scanned is how many blobs rows were examined.
	Scanned int
	// Truncated is true when the scan stopped at the row cap before the end of
	// the blobs table, so the totals are a lower bound.
	Truncated bool
	Count     int64
	Bytes     int64
	Routes    []PlanRoute
	Policies  []PlanPolicy
	Sample    []PlanSample
}

// Plan reports what a mover pass would move now, without changing anything: it
// only reads (the blobs table and policies) and never touches a store or the
// journal. With draft set only that policy is simulated; otherwise every
// enabled saved policy plus promote-on-read is. Unlike a pass it is not capped
// by the batch size, only by MaxScanRows.
func (e *Engine) Plan(ctx context.Context, draft *Policy, sampleLimit int) (*Plan, error) {
	if sampleLimit <= 0 || sampleLimit > MaxPlanSample {
		sampleLimit = MaxPlanSample
	}

	snap := e.reg.Snapshot()

	var policies []*Policy

	if draft != nil {
		d := *draft
		d.Enabled = true
		policies = usablePolicies(snap, []*Policy{&d})
	} else {
		var err error
		if policies, err = e.loadPolicies(ctx, snap); err != nil {
			return nil, err
		}
	}

	plan := &Plan{}
	acc := newPlanAccumulator(snap, plan, sampleLimit)

	visit := func(ds []decision) int {
		for _, d := range ds {
			acc.add(d)
		}

		return len(ds)
	}

	next, scanned, err := e.scanPolicies(ctx, snap, policies, "", e.cfg.MaxScanRows, visit)
	if err != nil {
		return nil, err
	}

	plan.Scanned = scanned
	plan.Truncated = next != ""

	if draft == nil {
		pnext, pscanned, err := e.scanPromotions(ctx, snap, policies, "", e.cfg.MaxScanRows, visit)
		if err != nil {
			return nil, err
		}

		plan.Scanned += pscanned
		plan.Truncated = plan.Truncated || pnext != ""
	}

	acc.finish()

	return plan, nil
}

type routeKey struct{ from, to string }

type planAccumulator struct {
	snap     *Snapshot
	plan     *Plan
	limit    int
	routes   map[routeKey]*PlanRoute
	policies map[string]*PlanPolicy
}

func newPlanAccumulator(snap *Snapshot, plan *Plan, limit int) *planAccumulator {
	return &planAccumulator{
		snap: snap, plan: plan, limit: limit,
		routes: map[routeKey]*PlanRoute{}, policies: map[string]*PlanPolicy{},
	}
}

func (a *planAccumulator) name(backend string) string {
	if t, ok := a.snap.ByBackend(backend); ok {
		return t.Name
	}

	return "unknown"
}

func (a *planAccumulator) add(d decision) {
	from, to := a.name(d.from), a.name(d.to)

	a.plan.Count++
	a.plan.Bytes += d.facts.SizeBytes

	rk := routeKey{from, to}
	if a.routes[rk] == nil {
		a.routes[rk] = &PlanRoute{From: from, To: to}
	}

	a.routes[rk].Count++
	a.routes[rk].Bytes += d.facts.SizeBytes

	if a.policies[d.policy] == nil {
		a.policies[d.policy] = &PlanPolicy{Policy: d.policy}
	}

	a.policies[d.policy].Count++
	a.policies[d.policy].Bytes += d.facts.SizeBytes

	if len(a.plan.Sample) < a.limit {
		a.plan.Sample = append(a.plan.Sample, PlanSample{
			SHA256: d.facts.SHA256, SizeBytes: d.facts.SizeBytes, From: from, To: to, Policy: d.policy,
			CreatedAt: d.facts.CreatedAt, LastAccessedAt: d.facts.LastAccessedAt,
		})
	}
}

// finish flattens the accumulators into deterministic, sorted slices.
func (a *planAccumulator) finish() {
	for _, r := range a.routes {
		a.plan.Routes = append(a.plan.Routes, *r)
	}

	sort.Slice(a.plan.Routes, func(i, j int) bool {
		x, y := a.plan.Routes[i], a.plan.Routes[j]
		if x.From != y.From {
			return x.From < y.From
		}

		return x.To < y.To
	})

	for _, p := range a.policies {
		a.plan.Policies = append(a.plan.Policies, *p)
	}

	sort.Slice(a.plan.Policies, func(i, j int) bool { return a.plan.Policies[i].Policy < a.plan.Policies[j].Policy })
}
