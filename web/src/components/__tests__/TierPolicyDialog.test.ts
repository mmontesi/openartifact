// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, it, expect, spyOn, afterEach, type Mock } from 'bun:test'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import TierPolicyDialog from '../TierPolicyDialog.vue'
import type { StorageTier } from '../../api/tiering'

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const TIERS: StorageTier[] = [
  { id: '1', name: 'hot', primary: true, rank: 0 },
  { id: '2', name: 'cold', primary: false, rank: 10 },
]
const DRY_RUN = {
  scanned: 10,
  truncated: false,
  would_move: { count: 3, bytes: 4096 },
  routes: [{ from: 'hot', to: 'cold', count: 3, bytes: 4096 }],
  policies: [{ policy: 'archive', count: 3, bytes: 4096 }],
  sample: [],
}

let wrapper: VueWrapper | undefined
let fetchSpy: Mock<typeof fetch>

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

async function mountDialog(): Promise<void> {
  fetchSpy = spyOn(globalThis, 'fetch').mockImplementation(async (url, init) => {
    if (String(url).endsWith('/dry-run')) return json(DRY_RUN)
    return json({ id: 'p1', ...JSON.parse(String(init?.body ?? '{}')) }, init?.method === 'POST' ? 201 : 200)
  })
  wrapper = mount(TierPolicyDialog, {
    props: { open: true, policy: null, tiers: TIERS },
    global: { plugins: [createPinia()] },
    attachTo: document.body,
  })
  await flushPromises()
}

function field(id: string): HTMLInputElement {
  return document.body.querySelector<HTMLInputElement>(`#${id}`)!
}

async function type(id: string, value: string): Promise<void> {
  const el = field(id)
  el.value = value
  el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }))
  await flushPromises()
}

function click(testId: string): void {
  document.body.querySelector<HTMLButtonElement>(`[data-testid="${testId}"]`)!.click()
}

function bodies(match: (url: string) => boolean): unknown[] {
  return fetchSpy.mock.calls.filter(([url]) => match(String(url))).map(([, init]) => JSON.parse(String(init?.body)))
}

describe('TierPolicyDialog', () => {
  it('blocks simulate and save until the draft is valid, explaining why', async () => {
    await mountDialog()
    expect(document.body.querySelector('[data-testid="tp-form-error"]')?.textContent).toContain('Enter a name')
    expect(document.body.querySelector('[data-testid="tp-simulate"]')?.hasAttribute('disabled')).toBe(true)
    expect(document.body.querySelector('[data-testid="tp-save"]')?.hasAttribute('disabled')).toBe(true)

    await type('tp-name', 'archive')
    expect(document.body.querySelector('[data-testid="tp-form-error"]')?.textContent).toContain('at least one condition')
  })

  it('simulates the draft with the age converted from days to seconds', async () => {
    await mountDialog()
    await type('tp-name', 'archive')
    await type('tp-age', '30')
    await type('tp-min-size', '10')

    click('tp-simulate')
    await flushPromises()

    const [body] = bodies((u) => u.endsWith('/dry-run')) as Array<{ policy: Record<string, unknown>; sample_limit: number }>
    expect(body!.policy).toMatchObject({
      name: 'archive',
      target_tier: 'cold',
      from_tier: null,
      min_age_seconds: 30 * 86_400,
      min_size_bytes: 10 * 1024 ** 3,
      formats: [],
    })
    expect(body!.sample_limit).toBeGreaterThan(0)
    expect(document.body.querySelector('[data-testid="dry-run-no-changes"]')?.textContent).toContain('No changes were made')
    expect(document.body.querySelector('[data-testid="dry-run-count"]')?.textContent?.trim()).toBe('3')
  })

  it('drops a simulation result once the draft is edited', async () => {
    await mountDialog()
    await type('tp-name', 'archive')
    await type('tp-age', '30')
    click('tp-simulate')
    await flushPromises()
    expect(document.body.querySelector('[data-testid="dry-run-result"]')).not.toBeNull()

    await type('tp-age', '60')
    expect(document.body.querySelector('[data-testid="dry-run-result"]')).toBeNull()
  })

  it('creates the policy with age and idle in seconds and the picked formats', async () => {
    await mountDialog()
    await type('tp-name', 'archive')
    await type('tp-age', '30')
    await type('tp-idle', '7')
    await type('tp-repo', 'team-*')
    click('tp-format-maven')
    await flushPromises()

    document.getElementById('tier-policy-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    const post = fetchSpy.mock.calls.find(([url, init]) => String(url) === '/api/v1/admin/tiering/policies' && init?.method === 'POST')
    expect(post).toBeDefined()
    expect(JSON.parse(String(post![1]!.body))).toMatchObject({
      name: 'archive',
      enabled: true,
      target_tier: 'cold',
      min_age_seconds: 30 * 86_400,
      min_idle_seconds: 7 * 86_400,
      repo_glob: 'team-*',
      formats: ['maven'],
    })
    expect(wrapper!.emitted('saved')).toHaveLength(1)
  })
})
