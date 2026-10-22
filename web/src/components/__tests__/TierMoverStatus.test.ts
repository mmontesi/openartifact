// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, it, expect, spyOn } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import TierMoverStatus from '../TierMoverStatus.vue'

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const PASS = (finished: string) => ({
  started_at: '2026-01-01T00:00:00Z',
  finished_at: finished,
  recovered: 0,
  promoted: 1,
  moved: 7,
  skipped: 2,
  failed: 0,
  bytes: 2048,
  scanned: 100,
  aborted: false,
})

async function until(cond: () => boolean): Promise<void> {
  const deadline = Date.now() + 2000
  while (!cond()) {
    if (Date.now() > deadline) throw new Error('condition not met in time')
    await new Promise((r) => setTimeout(r, 2))
    await flushPromises()
  }
}

function mountStatus(props: { pollIntervalMs?: number; maxPolls?: number } = {}) {
  return mount(TierMoverStatus, { props: { pollIntervalMs: 2, ...props }, global: { plugins: [createPinia()] } })
}

describe('TierMoverStatus', () => {
  it('shows the last pass and pending moves, and disables Run now when the mover is off', async () => {
    spyOn(globalThis, 'fetch').mockImplementation((async () =>
      json({
        enabled: false,
        running: false,
        interval_seconds: 300,
        last_pass: PASS('2026-01-01T00:01:00Z'),
        pending_total: 1,
        pending: [{ sha256: 'c'.repeat(64), size_bytes: 10, from: 'hot', to: 'cold', state: 'copying', reason: 'timeout', attempts: 2, updated_at: '2026-01-01T00:00:30Z' }],
      })) as never)
    const wrapper = mountStatus()
    await flushPromises()

    expect(wrapper.get('[data-testid="mover-last-pass"]').text()).toContain('7')
    expect(wrapper.get('[data-testid="mover-pending"]').text()).toContain('hot → cold')
    expect(wrapper.get('[data-testid="mover-run"]').attributes('disabled')).toBeDefined()
  })

  it('requests a pass, polls until a newer pass finished, then stops and notifies', async () => {
    let statusCalls = 0
    const fetchSpy = spyOn(globalThis, 'fetch').mockImplementation((async (url: string) => {
      if (String(url).endsWith('/run')) return json({ started: true }, 202)
      statusCalls++
      const base = { enabled: true, running: false, interval_seconds: 300, pending_total: 0, pending: [] }
      if (statusCalls === 1) return json({ ...base, last_pass: PASS('2026-01-01T00:01:00Z') })
      if (statusCalls === 2) return json({ ...base, running: true, last_pass: PASS('2026-01-01T00:01:00Z') })
      return json({ ...base, last_pass: PASS('2026-01-01T00:09:00Z') })
    }) as never)

    const wrapper = mountStatus()
    await flushPromises()
    await wrapper.get('[data-testid="mover-run"]').trigger('click')
    await until(() => wrapper.emitted('pass-finished') !== undefined)

    const callsAtFinish = statusCalls
    await new Promise((r) => setTimeout(r, 20))
    expect(statusCalls).toBe(callsAtFinish)
    expect(wrapper.emitted('pass-finished')).toHaveLength(1)
    expect(fetchSpy.mock.calls.some(([u, i]) => String(u).endsWith('/run') && i?.method === 'POST')).toBe(true)
  })

  it('gives up after maxPolls instead of polling forever', async () => {
    let statusCalls = 0
    spyOn(globalThis, 'fetch').mockImplementation((async (url: string) => {
      if (String(url).endsWith('/run')) return json({ started: true }, 202)
      statusCalls++
      return json({ enabled: true, running: false, interval_seconds: 300, pending_total: 0, pending: [], last_pass: PASS('2026-01-01T00:01:00Z') })
    }) as never)

    const wrapper = mountStatus({ maxPolls: 3 })
    await flushPromises()
    await wrapper.get('[data-testid="mover-run"]').trigger('click')
    await until(() => statusCalls >= 4)
    await new Promise((r) => setTimeout(r, 20))
    expect(statusCalls).toBe(4)
    expect(wrapper.emitted('pass-finished')).toBeUndefined()
  })

  it('stops polling when unmounted', async () => {
    let statusCalls = 0
    spyOn(globalThis, 'fetch').mockImplementation((async (url: string) => {
      if (String(url).endsWith('/run')) return json({ started: true }, 202)
      statusCalls++
      return json({ enabled: true, running: true, interval_seconds: 300, pending_total: 0, pending: [], last_pass: PASS('2026-01-01T00:01:00Z') })
    }) as never)

    const wrapper = mountStatus()
    await flushPromises()
    await until(() => statusCalls >= 3)
    wrapper.unmount()
    const atUnmount = statusCalls
    await new Promise((r) => setTimeout(r, 20))
    expect(statusCalls).toBeLessThanOrEqual(atUnmount + 1)
  })
})
