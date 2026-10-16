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

import type { AdminOrdersListParams, TopupOrder } from '../types'

export const DEFAULT_ORDERS_LIMIT = 50

/** Cap mirrors backend: limit <= 0 or > 100 → 50. */
export function clampOrdersLimit(limit: number | undefined | null): number {
  const n = Number(limit)
  if (!Number.isFinite(n) || n <= 0 || n > 100) return DEFAULT_ORDERS_LIMIT
  return Math.floor(n)
}

export function clampOrdersOffset(offset: number | undefined | null): number {
  const n = Number(offset)
  if (!Number.isFinite(n) || n < 0) return 0
  return Math.floor(n)
}

/**
 * Build query object for GET /api/admin/orders.
 * Omits empty filters; normalizes limit/offset.
 */
export function buildAdminOrdersQuery(
  params: AdminOrdersListParams
): Record<string, string | number> {
  const out: Record<string, string | number> = {
    limit: clampOrdersLimit(params.limit),
    offset: clampOrdersOffset(params.offset),
  }
  const status = (params.status || '').trim()
  if (status) out.status = status
  const orderNo = (params.order_no || '').trim()
  if (orderNo) out.order_no = orderNo
  if (params.user_id != null && params.user_id > 0) {
    out.user_id = Math.floor(params.user_id)
  }
  const method = (params.payment_method || '').trim()
  if (method) out.payment_method = method
  const provider = (params.payment_provider || '').trim()
  if (provider) out.payment_provider = provider
  if (params.create_time_from != null && params.create_time_from > 0) {
    out.create_time_from = params.create_time_from
  }
  if (params.create_time_to != null && params.create_time_to > 0) {
    out.create_time_to = params.create_time_to
  }
  return out
}

/** pending and expired (late payment) are completable; backend is authoritative (409 otherwise). */
export function canCompleteOrder(
  order: Pick<TopupOrder, 'status'> | null | undefined
): boolean {
  if (!order) return false
  return order.status === 'pending' || order.status === 'expired'
}

export function orderStatusLabelKey(status: string): string {
  switch (status) {
    case 'pending':
      return 'Pending'
    case 'paid':
      return 'Paid'
    case 'failed':
      return 'Failed'
    case 'expired':
      return 'Expired'
    case 'need_manual':
      return 'Needs manual review'
    default:
      return status || 'Unknown'
  }
}

export function orderStatusVariant(
  status: string
): 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'paid':
      return 'secondary'
    case 'failed':
      return 'destructive'
    case 'pending':
    default:
      return 'outline'
  }
}

export function refundStatusLabelKey(status: string): string {
  switch (status) {
    case 'none':
      return 'Refund none'
    case 'partial':
      return 'Refund partial'
    case 'full':
      return 'Refund full'
    default:
      return status || 'Unknown'
  }
}

export function refundStatusVariant(
  status: string
): 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'full':
      return 'destructive'
    case 'partial':
      return 'outline'
    case 'none':
    default:
      return 'secondary'
  }
}

/** Unix seconds → local locale string; 0/empty → em dash. */
export function formatUnixTime(
  unix: number | null | undefined,
  locale?: string
): string {
  if (unix == null || !Number.isFinite(Number(unix)) || Number(unix) <= 0) {
    return '—'
  }
  const ms = Number(unix) * 1000
  try {
    return new Date(ms).toLocaleString(locale)
  } catch {
    return new Date(ms).toLocaleString()
  }
}

export function formatUsd(n: number | null | undefined): string {
  if (n == null || Number.isNaN(Number(n))) return '—'
  return `$${Number(n).toFixed(2)}`
}

/** Backend payment_method (alipay | wxpay | usdt) → i18n key; unknown raw. */
export function paymentMethodLabelKey(method: string): string {
  switch (method) {
    case 'alipay':
      return 'Alipay'
    case 'wxpay':
      return 'WeChat Pay'
    case 'usdt':
      return 'USDT'
    default:
      return method
  }
}

/** Backend payment_provider (epay | bepusdt) → i18n key; unknown raw. */
export function paymentProviderLabelKey(provider: string): string {
  switch (provider) {
    case 'epay':
      return 'Epay'
    case 'bepusdt':
      return 'BEpusdt'
    default:
      return provider
  }
}

export function paymentLabel(
  order: {
    payment_method?: string
    payment_provider?: string
  },
  translate: (key: string) => string = (k) => k
): string {
  const method = (order.payment_method || '').trim()
  const provider = (order.payment_provider || '').trim()
  const m = method ? translate(paymentMethodLabelKey(method)) : ''
  const p = provider ? translate(paymentProviderLabelKey(provider)) : ''
  if (m && p && method !== provider) {
    return `${m} / ${p}`
  }
  return m || p || '—'
}

/** Map complete API failure to a stable English key or fallback. */
export function completeErrorMessage(
  status: number | undefined,
  code: string | undefined,
  fallback: string
): string {
  if (code === 'trade_no_required') return 'Gateway trade number is required'
  if (code === 'self_complete_forbidden') {
    return 'You cannot complete your own order'
  }
  if (code === 'no_payment_attempt') {
    return 'This order never started a real payment; cannot complete'
  }
  if (code === 'trade_no_duplicate') {
    return 'This trade number is already used by another order'
  }
  if (status === 403) return 'Insufficient permissions'
  if (status === 404 || code === 'not_found') return 'Order not found'
  if (status === 409) return 'Order status does not allow complete'
  if (code === 'confirm_required') return 'Second confirmation is required'
  return fallback
}

export function parseUserIdFilter(raw: string): number | undefined {
  const s = raw.trim()
  if (!s) return undefined
  const n = Number(s)
  if (!Number.isInteger(n) || n <= 0) return undefined
  return n
}

/** Display name for an order's user: username, or `#id` when the user is gone. */
export function orderUserDisplay(
  order: Pick<TopupOrder, 'user_id' | 'username'>
): string {
  const name = (order.username || '').trim()
  return name || `#${order.user_id}`
}
