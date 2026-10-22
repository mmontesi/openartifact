// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, it, expect } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import TierDryRunResult from '../TierDryRunResult.vue'
import type { TierDryRun } from '../../api/tiering'

const SHA = 'a'.repeat(64)
const RESULT: TierDryRun = {
  scanned: 1200,
  truncated: false,
  would_move: { count: 42, bytes: 3 * 1024 ** 3 },
  routes: [{ from: 'hot', to: 'cold', count: 42, bytes: 3 * 1024 ** 3 }],
  policies: [{ policy: 'archive', count: 42, bytes: 3 * 1024 ** 3 }],
  sample: [
    { sha256: SHA, size_bytes: 2048, from: 'hot', to: 'cold', policy: 'archive', created_at: '2026-01-01T00:00:00Z', last_accessed_at: null },
    { sha256: 'b'.repeat(64), size_bytes: 1024, from: 'hot', to: 'cold', policy: 'archive', created_at: '2026-01-01T00:00:00Z', last_accessed_at: '2026-02-01T00:00:00Z' },
  ],
}

const mountResult = (result: TierDryRun) => mount(TierDryRunResult, { props: { result }, global: { plugins: [createPinia()] } })

describe('TierDryRunResult', () => {
  it('shows the totals, routes, policies and sample', () => {
    const wrapper = mountResult(RESULT)
    expect(wrapper.get('[data-testid="dry-run-count"]').text()).toBe('42')
    expect(wrapper.get('[data-testid="dry-run-bytes"]').text()).toBe('3.0 GB')
    expect(wrapper.get('[data-testid="dry-run-route"]').text()).toContain('hot → cold')
    expect(wrapper.get('[data-testid="dry-run-policy"]').text()).toContain('archive')
    const rows = wrapper.findAll('[data-testid="dry-run-sample"]')
    expect(rows).toHaveLength(2)
    // Short sha with the full value in the tooltip.
    const sha = rows[0]!.get('button')
    expect(sha.text()).toBe('a'.repeat(12))
    expect(sha.attributes('title')).toBe(SHA)
    expect(rows[0]!.text()).toContain('Never')
    expect(rows[1]!.text()).not.toContain('Never')
  })

  it('always states that nothing was changed', () => {
    expect(mountResult(RESULT).get('[data-testid="dry-run-no-changes"]').text()).toContain('No changes were made')
  })

  it('warns that totals are a lower bound when the scan was truncated', () => {
    expect(mountResult(RESULT).find('[data-testid="dry-run-truncated"]').exists()).toBe(false)
    const wrapper = mountResult({ ...RESULT, truncated: true })
    expect(wrapper.get('[data-testid="dry-run-truncated"]').text()).toContain('lower bound')
  })

  it('renders an empty simulation without tables', () => {
    const wrapper = mountResult({ scanned: 0, truncated: false, would_move: { count: 0, bytes: 0 }, routes: [], policies: [], sample: [] })
    expect(wrapper.get('[data-testid="dry-run-count"]').text()).toBe('0')
    expect(wrapper.find('table').exists()).toBe(false)
  })
})
