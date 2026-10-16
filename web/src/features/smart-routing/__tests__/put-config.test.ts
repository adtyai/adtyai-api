import { beforeEach, describe, expect, it, vi } from 'vitest'

const put = vi.fn()
vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put, post: vi.fn(), delete: vi.fn() },
}))

const { putRoutingConfig } = await import('../api')

describe('putRoutingConfig (audit P1-3 merge contract)', () => {
  beforeEach(() => {
    put.mockReset()
    put.mockResolvedValue({ data: { success: true, data: {} } })
  })

  it('toggle sends only {enabled}', async () => {
    await putRoutingConfig({ enabled: false })
    expect(put.mock.calls[0][1]).toEqual({ enabled: false })
  })

  it('weight apply sends backend weight keys + token only', async () => {
    await putRoutingConfig({
      weights: {
        connectivity: 40,
        price: 20,
        speed: 20,
        stability: 10,
        authenticity: 10,
      } as never,
      preview_token: 'tok',
    })
    const body = put.mock.calls[0][1]
    expect(Object.keys(body).sort()).toEqual(['preview_token', 'weights'])
    expect(body.weights).toMatchObject({ conn: 40, stab: 10, auth: 10 })
  })

  it('refuses an empty patch', async () => {
    const res = await putRoutingConfig({ preview_token: 'tok' })
    expect(res.success).toBe(false)
    expect(put).not.toHaveBeenCalled()
  })
})
