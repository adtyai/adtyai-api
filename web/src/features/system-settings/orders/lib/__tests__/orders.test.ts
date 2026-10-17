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

import {
  buildAdminOrdersQuery,
  canCompleteOrder,
  clampOrdersLimit,
  clampOrdersOffset,
  completeErrorMessage,
  formatUnixTime,
  formatUsd,
  orderStatusLabelKey,
  orderStatusVariant,
  parseUserIdFilter,
  paymentLabel,
  refundStatusLabelKey,
} from '../orders'

describe('clampOrdersLimit / offset', () => {
  it('defaults invalid or oversized limit to 50', () => {
    expect(clampOrdersLimit(undefined)).toBe(50)
    expect(clampOrdersLimit(0)).toBe(50)
    expect(clampOrdersLimit(101)).toBe(50)
    expect(clampOrdersLimit(50)).toBe(50)
    expect(clampOrdersLimit(20)).toBe(20)
  })

  it('clamps negative offset to 0', () => {
    expect(clampOrdersOffset(-1)).toBe(0)
    expect(clampOrdersOffset(10)).toBe(10)
  })
})

describe('buildAdminOrdersQuery', () => {
  it('always sends limit and offset', () => {
    expect(buildAdminOrdersQuery({})).toEqual({ limit: 50, offset: 0 })
  })

  it('includes trimmed filters only when set', () => {
    expect(
      buildAdminOrdersQuery({
        status: ' pending ',
        order_no: ' ADTY1 ',
        user_id: 7,
        payment_method: '',
        limit: 20,
        offset: 40,
      })
    ).toEqual({
      limit: 20,
      offset: 40,
      status: 'pending',
      order_no: 'ADTY1',
      user_id: 7,
    })
  })

  it('omits user_id when not positive', () => {
    const q = buildAdminOrdersQuery({ user_id: 0 })
    expect(q.user_id).toBeUndefined()
  })
})

describe('canCompleteOrder', () => {
  it('true for pending and expired (late payment)', () => {
    expect(canCompleteOrder({ status: 'pending' })).toBe(true)
    expect(canCompleteOrder({ status: 'expired' })).toBe(true)
    expect(orderStatusLabelKey('expired')).toBe('Expired')
    expect(canCompleteOrder({ status: 'paid' })).toBe(false)
    expect(canCompleteOrder({ status: 'failed' })).toBe(false)
    expect(canCompleteOrder(null)).toBe(false)
  })
})

describe('status badges', () => {
  it('maps order status keys and variants', () => {
    expect(orderStatusLabelKey('pending')).toBe('Pending')
    expect(orderStatusLabelKey('paid')).toBe('Paid')
    expect(orderStatusLabelKey('failed')).toBe('Failed')
    expect(orderStatusVariant('paid')).toBe('secondary')
    expect(orderStatusVariant('failed')).toBe('destructive')
    expect(orderStatusVariant('pending')).toBe('outline')
  })

  it('maps refund status keys', () => {
    expect(refundStatusLabelKey('none')).toBe('Refund none')
    expect(refundStatusLabelKey('partial')).toBe('Refund partial')
    expect(refundStatusLabelKey('full')).toBe('Refund full')
  })

  it('refund status covers backend enum (model/topup_order.go) with zh names', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const zh = JSON.parse(
      readFileSync(resolve(process.cwd(), 'src/i18n/locales/zh.json'), 'utf8')
    ).translation as Record<string, string>
    const src = readFileSync(
      resolve(process.cwd(), '../model/topup_order.go'),
      'utf8'
    )
    const values = [...src.matchAll(/RefundStatus\w+\s*=\s*"([a-z_]+)"/g)].map(
      (m) => m[1]
    )
    expect(values.sort()).toEqual(['full', 'none', 'partial'])
    expect(values.map((v) => zh[refundStatusLabelKey(v)]).sort()).toEqual([
      '已退款',
      '未退款',
      '部分退款',
    ])
    expect(refundStatusLabelKey('processing')).toBe('processing')
  })
})

describe('format helpers', () => {
  it('formatUsd', () => {
    expect(formatUsd(12.5)).toBe('$12.50')
    expect(formatUsd(null)).toBe('—')
  })

  it('formatUnixTime', () => {
    expect(formatUnixTime(0)).toBe('—')
    expect(formatUnixTime(null)).toBe('—')
    // 2026-10-15 00:00:00 UTC
    const s = formatUnixTime(1760486400, 'en-US')
    expect(s).not.toBe('—')
    expect(s.length).toBeGreaterThan(4)
  })

  it('paymentLabel', () => {
    expect(
      paymentLabel({ payment_method: 'alipay', payment_provider: 'epay' })
    ).toBe('Alipay / Epay')
    expect(paymentLabel({ payment_method: 'usdt' })).toBe('USDT')
    expect(
      paymentLabel(
        { payment_method: 'wxpay', payment_provider: 'epay' },
        (k) => ({ 'WeChat Pay': '微信支付', Epay: '易支付' })[k] ?? k
      )
    ).toBe('微信支付 / 易支付')
    expect(
      paymentLabel({ payment_method: 'usdt', payment_provider: 'bepusdt' })
    ).toBe('USDT / BEpusdt')
    expect(paymentLabel({ payment_method: 'crypto_x' })).toBe('crypto_x')
    expect(paymentLabel({})).toBe('—')
  })
})

describe('completeErrorMessage', () => {
  it('maps 403/404/409', () => {
    expect(completeErrorMessage(403, undefined, 'x')).toBe(
      'Insufficient permissions'
    )
    expect(completeErrorMessage(404, undefined, 'x')).toBe('Order not found')
    expect(completeErrorMessage(409, undefined, 'x')).toBe(
      'Order status does not allow complete'
    )
    expect(completeErrorMessage(400, 'confirm_required', 'x')).toBe(
      'Confirmation required'
    )
    expect(completeErrorMessage(500, undefined, 'boom')).toBe('boom')
    expect(completeErrorMessage(400, 'trade_no_required', 'x')).toBe(
      'Gateway trade number is required'
    )
    expect(completeErrorMessage(403, 'self_complete_forbidden', 'x')).toBe(
      'You cannot complete your own order'
    )
    expect(completeErrorMessage(409, 'no_payment_attempt', 'x')).toBe(
      'This order never started a real payment; cannot complete'
    )
    expect(completeErrorMessage(409, 'trade_no_duplicate', 'x')).toBe(
      'This trade number is already used by another order'
    )
  })
})

describe('parseUserIdFilter', () => {
  it('parses positive integers only', () => {
    expect(parseUserIdFilter('12')).toBe(12)
    expect(parseUserIdFilter('')).toBeUndefined()
    expect(parseUserIdFilter('0')).toBeUndefined()
    expect(parseUserIdFilter('1.5')).toBeUndefined()
    expect(parseUserIdFilter('abc')).toBeUndefined()
  })
})

describe('orderUserDisplay', () => {
  it('prefers username, falls back to #id', async () => {
    const { orderUserDisplay } = await import('../orders')
    expect(orderUserDisplay({ user_id: 7, username: 'alice' })).toBe('alice')
    expect(orderUserDisplay({ user_id: 7, username: '' })).toBe('#7')
    expect(orderUserDisplay({ user_id: 7 })).toBe('#7')
  })
})
