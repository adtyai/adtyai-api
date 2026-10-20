import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
vi.mock('@/lib/api', () => ({ api: { get } }))

const {
  fetchChatwootConfig,
  fetchChatwootIdentity,
  identityAttrs,
  isSafeChatwootBaseUrl,
} = await import('../api')

describe('chatwoot (N-P2-10)', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('returns config only when enabled with base_url + token', async () => {
    get.mockResolvedValueOnce({
      data: {
        success: true,
        data: { enabled: true, base_url: 'https://cw', website_token: 'tok' },
      },
    })
    expect(await fetchChatwootConfig()).toMatchObject({
      base_url: 'https://cw',
    })
    get.mockResolvedValueOnce({
      data: {
        success: true,
        data: { enabled: false, base_url: '', website_token: '' },
      },
    })
    expect(await fetchChatwootConfig()).toBeNull()
  })

  it('rejects unsafe base URLs before the widget can load a script', async () => {
    expect(isSafeChatwootBaseUrl('https://cw.example.com')).toBe(true)
    expect(isSafeChatwootBaseUrl('http://127.0.0.1:3000')).toBe(true)
    expect(isSafeChatwootBaseUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeChatwootBaseUrl('http://evil.example')).toBe(false)
    expect(isSafeChatwootBaseUrl('https://user:pass@cw.example.com')).toBe(false)

    get.mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          enabled: true,
          base_url: 'javascript:alert(1)',
          website_token: 'tok',
        },
      },
    })
    expect(await fetchChatwootConfig()).toBeNull()
  })

  it('soft-fails on network error', async () => {
    get.mockRejectedValue(new Error('down'))
    expect(await fetchChatwootConfig()).toBeNull()
    expect(await fetchChatwootIdentity()).toBeNull()
  })

  it('passes identifier_hash to setUser attrs', async () => {
    get.mockResolvedValueOnce({
      data: {
        success: true,
        data: { identifier: '7', identifier_hash: 'abc', name: 'n', email: '' },
      },
    })
    const id = await fetchChatwootIdentity()
    expect(id?.identifier).toBe('7')
    expect(identityAttrs(id as never)).toEqual({
      name: 'n',
      identifier_hash: 'abc',
    })
  })
})

describe('chatwoot unverified identity (R-P2-9)', () => {
  it('drops email and hash when identity_verified=false', async () => {
    const { identityAttrs } = await import('../api')
    expect(
      identityAttrs({
        identifier: '7',
        identity_verified: false,
        email: 'a@b.c',
        identifier_hash: 'x',
        name: 'n',
      })
    ).toEqual({ name: 'n' })
  })
})
