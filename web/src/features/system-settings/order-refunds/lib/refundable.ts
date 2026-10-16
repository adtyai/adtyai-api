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

import type { TopupOrder } from '../types'

/** Order credit base used for refund hint (prefer credit_amount_usd). */
export function orderCreditUsd(
  order: Pick<TopupOrder, 'credit_amount_usd' | 'paid_amount_usd'>
): number {
  const credit = Number(order.credit_amount_usd)
  if (Number.isFinite(credit) && credit > 0) return credit
  const paid = Number(order.paid_amount_usd)
  return Number.isFinite(paid) && paid > 0 ? paid : 0
}

/** Frontend hint only; backend refundable = min(orderRemain, user paid balance). */
export function refundHintUsd(order: TopupOrder): number {
  const base = orderCreditUsd(order)
  const refunded = Number(order.refunded_amount) || 0
  const remain = base - (refunded > 0 ? refunded : 0)
  return remain > 0 ? remain : 0
}

export function canRefundOrder(order: TopupOrder | null | undefined): boolean {
  if (!order) return false
  if (order.status !== 'paid') return false
  if (order.refund_status === 'full') return false
  return refundHintUsd(order) > 1e-9
}

export function newRefundRequestId(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID()
  }
  return `refund-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

export function refundErrorMessage(
  code: string | undefined,
  fallback: string
): string {
  switch (code) {
    case 'confirm_required':
      return 'Second confirmation is required'
    case 'request_id_required':
      return 'Request number is required'
    case 'reason_required':
      return 'Refund reason is required'
    case 'amount_invalid':
      return 'Amount must be greater than 0'
    case 'refund_exceeds_available':
      return 'Refund exceeds available amount'
    case 'refund_idempotent':
      return 'This request number was already used; do not resubmit'
    case 'not_found':
      return 'Order not found'
    case 'order_status_invalid':
      return 'Order status does not allow refund'
    default:
      return fallback
  }
}
