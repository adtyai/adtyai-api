import { describe, expect, it, vi } from 'vitest'

const post = vi.fn()
vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post, delete: vi.fn() },
}))

const { envelopeFromError, previewWeightScheme } = await import('../api')
const { routingErrorText } = await import('../lib/error-text')

const axios400 = Object.assign(
  new Error('Request failed with status code 400'),
  {
    response: {
      status: 400,
      data: {
        success: false,
        code: 'preview_required',
        message: 'preview_token does not match requested change',
      },
    },
  }
)

describe('F-P2-1 backend reason on 4xx', () => {
  it('keeps backend code + message', () => {
    expect(envelopeFromError(axios400)).toMatchObject({
      success: false,
      code: 'preview_required',
      message: 'preview_token does not match requested change',
    })
  })
  it('maps code to readable text', () => {
    const t = ((k: string) => k) as never
    expect(routingErrorText(t, envelopeFromError(axios400), 'x')).toContain(
      'preview again'
    )
  })
})

describe('F2-P2-3 non-JSON 5xx keeps status', () => {
  it('html 502 → http_error with status', () => {
    const err = Object.assign(
      new Error('Request failed with status code 502'),
      {
        response: { status: 502, data: '<html>Bad Gateway</html>' },
      }
    )
    const env = envelopeFromError(err)
    expect(env).toMatchObject({ code: 'http_error', status: 502 })
    const t = ((k: string, o?: { status?: number }) =>
      k.replace('{{status}}', String(o?.status))) as never
    expect(routingErrorText(t, env, 'x')).toBe('Request failed (HTTP 502)')
  })
})

describe('F-P2-2 preview failure is explicit', () => {
  it('returns error instead of silent empty', async () => {
    post.mockRejectedValueOnce(axios400)
    const r = await previewWeightScheme({
      weights: {
        connectivity: 1,
        price: 1,
        speed: 1,
        stability: 1,
        authenticity: 1,
      } as never,
    })
    expect(r.items).toEqual([])
    expect(r.error_code).toBe('preview_required')
    expect(r.preview_token).toBeUndefined()
  })
})
