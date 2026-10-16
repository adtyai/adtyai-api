import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.fn()
const get = vi.fn()
vi.mock('@/lib/api', () => ({
  api: { get, put: vi.fn(), post, delete: vi.fn() },
}))

const { refreshChannelBalance, startCostBackfill, getCostBackfillStatus } =
  await import('../api')
const { routingErrorText } = await import('../lib/error-text')
const { ledgerKindLabelKey } =
  await import('@/features/wallet/ledger/lib/ledger')
const { getMessageErrorState, LOW_BALANCE_CONCURRENCY_CONTENT } =
  await import('@/features/playground/lib/message/message-error-utils')

const httpErr = (status: number, data: unknown) =>
  Object.assign(new Error(`Request failed with status code ${status}`), {
    response: { status, data },
  })
const t = ((k: string) => k) as never

describe('backend r2 contract (frontend r4)', () => {
  beforeEach(() => {
    post.mockReset()
    get.mockReset()
  })

  it('balance refresh: unsupported is a 200 status', async () => {
    post.mockResolvedValueOnce({
      data: { success: true, data: { channel_id: 3, status: 'unsupported' } },
    })
    expect((await refreshChannelBalance(3)).data?.status).toBe('unsupported')
  })

  it('balance refresh: 502 balance_query_failed → status failed', async () => {
    post.mockRejectedValueOnce(
      httpErr(502, {
        success: false,
        code: 'balance_query_failed',
        message: 'upstream 401',
      })
    )
    const r = await refreshChannelBalance(3)
    expect(r.data?.status).toBe('failed')
  })

  it('backfill: 202 job_id, 409 backfill_running, GET status', async () => {
    post.mockResolvedValueOnce({
      status: 202,
      data: { success: true, data: { job_id: 9, status: 'running' } },
    })
    expect((await startCostBackfill()).data?.job_id).toBe(9)
    post.mockRejectedValueOnce(
      httpErr(409, { success: false, code: 'backfill_running', message: 'x' })
    )
    expect((await startCostBackfill()).code).toBe('backfill_running')
    get.mockResolvedValueOnce({
      data: { success: true, data: { job_id: 9, status: 'done', inserted: 3 } },
    })
    expect((await getCostBackfillStatus())?.status).toBe('done')
  })

  it('maps the 4 new PUT /config codes and 429', () => {
    for (const code of [
      'unknown_field',
      'invalid_weights',
      'scheme_not_found',
      'scheme_weights_mismatch',
    ]) {
      expect(routingErrorText(t, { code, message: code }, 'fallback')).not.toBe(
        'fallback'
      )
    }
    expect(routingErrorText(t, { status: 429, message: 'x' }, 'f')).toBe(
      'Too many requests, please try again later'
    )
  })

  it('ledger consume_arrears label', () => {
    expect(ledgerKindLabelKey('consume_arrears')).toBe('Ledger kind arrears')
  })

  it('playground low-balance 429 message is localized', () => {
    const state = getMessageErrorState(
      {
        status: 'error',
        versions: [
          {
            id: 'v1',
            content:
              'too many concurrent requests while balance is low (max 2)',
          },
        ],
      } as never,
      false
    )
    expect(state?.content).toBe(LOW_BALANCE_CONCURRENCY_CONTENT)
  })
})
