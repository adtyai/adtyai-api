import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
const put = vi.fn()
vi.mock('@/lib/api', () => ({ api: { get, put, post: vi.fn(), delete: vi.fn() } }))

const { putRoutingConfig } = await import('../api')

const live = {
  enabled: true,
  balance_patrol_enabled: true,
  token_skew_suspect: 0.2,
  halfopen_stage2_stable_cycles: 3,
  active_weight_scheme: 'balanced_default',
  weights: { conn: 30, price: 20, speed: 20, stab: 20, auth: 10 },
}

describe('putRoutingConfig (audit P1-3)', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
    get.mockResolvedValue({ data: { success: true, data: live } })
    put.mockResolvedValue({ data: { success: true, data: live } })
  })

  it('toggle sends full live config with only enabled changed', async () => {
    await putRoutingConfig({ enabled: false })
    const body = put.mock.calls[0][1]
    expect(body).toEqual({ ...live, enabled: false })
  })

  it('weight apply keeps every other field', async () => {
    await putRoutingConfig({
      weights: { connectivity: 40, price: 20, speed: 20, stability: 10, authenticity: 10 } as never,
      preview_token: 'tok',
    })
    const body = put.mock.calls[0][1]
    expect(body.enabled).toBe(true)
    expect(body.balance_patrol_enabled).toBe(true)
    expect(body.halfopen_stage2_stable_cycles).toBe(3)
    expect(body.preview_token).toBe('tok')
    expect(body.active_weight_scheme).toBe('balanced_default')
  })

  it('refuses to PUT when live config cannot be loaded', async () => {
    get.mockRejectedValue(new Error('down'))
    const res = await putRoutingConfig({ enabled: false })
    expect(res.success).toBe(false)
    expect(put).not.toHaveBeenCalled()
  })
})
