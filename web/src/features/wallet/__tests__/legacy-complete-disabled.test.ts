/*
Copyright (C) 2023-2026 QuantumNous
*/
import { describe, expect, it } from 'vitest'

import { completeOrder, requestPayment } from '../api'

describe('legacy wallet money paths disabled', () => {
  it('requestPayment throws and does not call /api/user/pay', async () => {
    await expect(
      requestPayment({ amount: 1, payment_method: 'epay' } as never)
    ).rejects.toThrow(/Legacy \/api\/user\/pay is disabled/)
  })

  it('completeOrder throws and does not call /api/user/topup/complete', async () => {
    await expect(completeOrder({ trade_no: 'x' })).rejects.toThrow(
      /Legacy \/api\/user\/topup\/complete is disabled/
    )
  })
})
