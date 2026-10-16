/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { describe, expect, it } from 'vitest'

import type { TopupOrder } from '../../types'
import {
  canRefundOrder,
  orderCreditUsd,
  refundErrorMessage,
  refundHintUsd,
} from '../refundable'

function order(partial: Partial<TopupOrder>): TopupOrder {
  return {
    id: 1,
    order_no: 'T1',
    user_id: 1,
    amount_usd: 10,
    paid_amount_usd: 10,
    credit_amount_usd: 10,
    bonus_amount_usd: 1,
    status: 'paid',
    refund_status: 'none',
    refunded_amount: 0,
    ...partial,
  }
}

describe('orderCreditUsd', () => {
  it('prefers credit_amount_usd', () => {
    expect(orderCreditUsd(order({ credit_amount_usd: 8, paid_amount_usd: 10 }))).toBe(8)
  })
  it('falls back to paid_amount_usd', () => {
    expect(orderCreditUsd(order({ credit_amount_usd: 0, paid_amount_usd: 10 }))).toBe(10)
  })
})

describe('refundHintUsd', () => {
  it('subtracts refunded_amount', () => {
    expect(refundHintUsd(order({ credit_amount_usd: 10, refunded_amount: 3 }))).toBe(7)
  })
  it('never negative', () => {
    expect(refundHintUsd(order({ credit_amount_usd: 10, refunded_amount: 12 }))).toBe(0)
  })
})

describe('canRefundOrder', () => {
  it('requires paid and not full', () => {
    expect(canRefundOrder(order({ status: 'pending' }))).toBe(false)
    expect(canRefundOrder(order({ refund_status: 'full' }))).toBe(false)
    expect(canRefundOrder(order({ refund_status: 'partial', refunded_amount: 2 }))).toBe(true)
    expect(canRefundOrder(order({}))).toBe(true)
  })
})

describe('refundErrorMessage', () => {
  it('maps known codes', () => {
    expect(refundErrorMessage('refund_exceeds_available', 'x')).toBe(
      'Refund exceeds available amount'
    )
    expect(refundErrorMessage('refund_idempotent', 'x')).toBe(
      'This request number was already used; do not resubmit'
    )
    expect(refundErrorMessage('mystery', 'fallback')).toBe('fallback')
  })
})
