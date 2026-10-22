// SPDX-FileCopyrightText: The OpenArtifact Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, it, expect, spyOn, type Mock } from 'bun:test'
import { createPolicy, createTier, deletePolicy, deleteTier, dryRun, getMoverStatus, listPolicies, listTiers, runMover, updatePolicy, updateTier } from '../tiering'

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

function stub(body: unknown = {}, status = 200): Mock<typeof fetch> {
  return spyOn(globalThis, 'fetch').mockImplementation(async () => (status === 204 ? new Response(null, { status }) : json(body, status)))
}

function lastCall(spy: Mock<typeof fetch>): { url: string; method: string; body: unknown } {
  const [url, init] = spy.mock.calls.at(-1)!
  return { url: String(url), method: init?.method ?? 'GET', body: init?.body ? JSON.parse(String(init.body)) : undefined }
}

describe('tiering api', () => {
  it('lists tiers and policies from the paged envelope', async () => {
    const spy = stub({ total: 1, items: [{ name: 'hot' }] })
    expect(await listTiers()).toEqual({ total: 1, items: [{ name: 'hot' }] })
    expect(lastCall(spy)).toMatchObject({ url: '/api/v1/admin/tiering/tiers', method: 'GET' })
    await listPolicies()
    expect(lastCall(spy).url).toBe('/api/v1/admin/tiering/policies')
  })

  it('creates, patches and deletes tiers with the tier name encoded', async () => {
    const spy = stub({ name: 'cold' }, 201)
    await createTier({ name: 'cold', backend: 's3-archive', rank: 20, promote_on_read: true })
    expect(lastCall(spy)).toEqual({
      url: '/api/v1/admin/tiering/tiers',
      method: 'POST',
      body: { name: 'cold', backend: 's3-archive', rank: 20, promote_on_read: true },
    })
    await updateTier('cold tier', { promote_on_read: false })
    expect(lastCall(spy)).toEqual({ url: '/api/v1/admin/tiering/tiers/cold%20tier', method: 'PATCH', body: { promote_on_read: false } })
    spy.mockRestore()
    const del = stub(undefined, 204)
    await deleteTier('cold')
    expect(lastCall(del)).toMatchObject({ url: '/api/v1/admin/tiering/tiers/cold', method: 'DELETE' })
  })

  it('creates, replaces and deletes policies', async () => {
    const spy = stub({ id: 'p1' }, 201)
    const input = { name: 'archive', target_tier: 'cold', min_age_seconds: 86_400 }
    await createPolicy(input)
    expect(lastCall(spy)).toEqual({ url: '/api/v1/admin/tiering/policies', method: 'POST', body: input })
    await updatePolicy('p1', input)
    expect(lastCall(spy)).toEqual({ url: '/api/v1/admin/tiering/policies/p1', method: 'PUT', body: input })
    spy.mockRestore()
    const del = stub(undefined, 204)
    await deletePolicy('p1')
    expect(lastCall(del)).toMatchObject({ url: '/api/v1/admin/tiering/policies/p1', method: 'DELETE' })
  })

  it('posts dry runs, mover runs and reads the status', async () => {
    const spy = stub({ scanned: 0 })
    await dryRun({ policy: { name: 'd', target_tier: 'cold', min_age_seconds: 1 }, sample_limit: 5 })
    expect(lastCall(spy)).toMatchObject({ url: '/api/v1/admin/tiering/dry-run', method: 'POST', body: { sample_limit: 5 } })
    await runMover()
    expect(lastCall(spy)).toMatchObject({ url: '/api/v1/admin/tiering/run', method: 'POST' })
    await getMoverStatus()
    expect(lastCall(spy)).toMatchObject({ url: '/api/v1/admin/tiering/status', method: 'GET' })
  })
})
