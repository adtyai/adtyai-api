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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

// Ported from the previous fork and reduced to USD: no CNY conversion and no
// subscription orders (adty task 3.5).

type ApiResponse<T> = { success: boolean; message?: string; data: T }

export type PageResponse<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type RechargeAuditOrder = {
  id: number
  user_id: number
  username: string
  site_id: number | null
  site_name: string
  order_type: 'topup'
  trade_no: string
  payment_method: string
  payment_provider: string
  payment_scene?: string
  payment_scene_name?: string
  payment_scene_version?: number
  status: string
  create_time: number
  complete_time: number
  amount: number
  money: number
  has_ext: boolean
  face_usd: string
  bonus_usd: string
  credit_quota: number
  pay_amount: string
  pay_currency: string
  paid_amount: string
  paid_currency: string
  paid_tx_hash: string
  paid_source: string
  channel_id: number
  channel_name: string
  anomaly: '' | 'missing_ext' | 'paid_mismatch'
  actions: { complete: boolean; refund: boolean; query: boolean }
}

export type RechargeAuditSummary = {
  orders: number
  credited_usd: string
  bonus_usd: string
  currency: 'USD'
  status_options: string[]
  provider_option: string[]
  site_options: { site_id: number; site_name: string }[]
  provider_capabilities: {
    provider: string
    label: string
    query: boolean
    complete: boolean
    refund: boolean
    refund_mode: 'offline'
  }[]
}

export type PaymentOrphanEvent = {
  id: number
  provider: string
  event_type: string
  reference_id: string
  status: string
  reason: string
  error: string
  create_time: number
  resolution_note: string
  can_credit: boolean
  review?: PaymentOrphanReview
}

export type PaymentOrphanReviewStatus =
  | 'pending_review'
  | 'refunded'
  | 'never_credited'
  | 'dismissed'

export type PaymentOrphanDismissalReason =
  | 'not_this_order'
  | 'reversal_canceled'

export type PaymentOrphanReview = {
  id: number
  status: PaymentOrphanReviewStatus
  note: string
  proof: string
  dismissal_reason?: PaymentOrphanDismissalReason
  trade_no: string
  external_amount: string
  external_currency: string
  refund_ledger_id: number
  deducted_quota: number
  refund_quota: number
  refund_amount: string
  paid_amount: string
  paid_currency: string
  exposure_amount: string
  created_at: number
  operator_id: number
}

export type PaymentOrphanReviewRequest = {
  status: PaymentOrphanReviewStatus
  trade_no: string
  external_amount: string
  external_currency: string
  proof: string
  note: string
  dismissal_reason?: PaymentOrphanDismissalReason
}

export type RechargeAuditFilters = {
  keyword?: string
  status?: string
  provider?: string
  method?: string
  site_id?: number
  order_type?: 'topup'
  page: number
  pageSize: number
}

function auditParams(filters: RechargeAuditFilters) {
  return {
    keyword: filters.keyword || undefined,
    status: filters.status || undefined,
    provider: filters.provider || undefined,
    method: filters.method || undefined,
    site_id: filters.site_id,
    order_type: filters.order_type || undefined,
    p: filters.page,
    page_size: filters.pageSize,
  }
}

export async function getRechargeAudit(
  filters: RechargeAuditFilters,
  ownerMode = false
) {
  const res = await api.get<ApiResponse<PageResponse<RechargeAuditOrder>>>(
    ownerMode
      ? '/api/adty/site/orders'
      : '/api/user/admin/finance/recharge-audit',
    { params: auditParams(filters) }
  )
  return requireServerSuccess(res.data).data
}

export async function getRechargeAuditSummary(
  filters: RechargeAuditFilters,
  ownerMode = false
) {
  const res = await api.get<ApiResponse<RechargeAuditSummary>>(
    ownerMode
      ? '/api/adty/site/orders/summary'
      : '/api/user/admin/finance/recharge-audit/summary',
    { params: auditParams(filters) }
  )
  return requireServerSuccess(res.data).data
}

export type OrderTraceSection =
  | 'payments'
  | 'wallet'
  | 'bonuses'
  | 'commissions'
  | 'commission_movements'
  | 'site_wallet'
  | 'exceptions'
  | 'reviews'

export type OrderTraceEntry = {
  id: number
  kind: string
  source: string
  status: string
  created_at: number
  reference: string
  amount?: string
  currency?: string
  delta_quota?: number
  balance_before?: string
  balance_after?: string
  pending_delta?: string
  available_delta?: string
  frozen_delta?: string
  withdrawn_delta?: string
  operator_id?: number
  note?: string
}

export type OrderTrace = PageResponse<OrderTraceEntry> & {
  order: RechargeAuditOrder
  refund: {
    refunded_at: number
    refund_quota: number
    refund_amount: string
    currency: string
    reason?: string
  }
  section: OrderTraceSection
  sections: OrderTraceSection[]
  promo_bonus_usd: string
}

export async function getOrderTrace(
  tradeNo: string,
  section: OrderTraceSection,
  page: number,
  ownerMode = false
) {
  const prefix = ownerMode
    ? '/api/adty/site/orders'
    : '/api/user/admin/finance/recharge-audit'
  const res = await api.get<ApiResponse<OrderTrace>>(
    `${prefix}/${encodeURIComponent(tradeNo)}`,
    { params: { section, p: page, page_size: 20 } }
  )
  return requireServerSuccess(res.data).data
}

export async function getPaymentOrphans(status: string, page: number) {
  const res = await api.get<ApiResponse<PageResponse<PaymentOrphanEvent>>>(
    '/api/user/admin/finance/payment-orphans',
    { params: { status, p: page, page_size: 20 } }
  )
  return requireServerSuccess(res.data).data
}

export async function creditPaymentOrphan(id: number, note: string) {
  const res = await api.post<ApiResponse<null>>(
    `/api/user/admin/finance/payment-orphans/${id}/credit`,
    { note }
  )
  requireServerSuccess(res.data)
}

export async function resolvePaymentOrphan(
  id: number,
  review: PaymentOrphanReviewRequest
) {
  const res = await api.post<ApiResponse<PaymentOrphanReview>>(
    `/api/user/admin/finance/payment-orphans/${id}/resolve`,
    review
  )
  return requireServerSuccess(res.data).data
}

export async function completeTopupManually(
  tradeNo: string,
  proof: string,
  note: string
) {
  const res = await api.post<ApiResponse<{ already_done: boolean }>>(
    `/api/adty/admin/topup/${encodeURIComponent(tradeNo)}/complete`,
    { proof, note }
  )
  return requireServerSuccess(res.data).data
}

export async function queryTopupPayment(tradeNo: string) {
  const res = await api.post<ApiResponse<{ status: string }>>(
    `/api/adty/admin/topup/${encodeURIComponent(tradeNo)}/query`
  )
  return requireServerSuccess(res.data).data
}

/** Refund result: quota taken back, principal quota refunded and the money to return offline. */
export type TopupRefundResult = {
  deducted_quota: number
  bonus_quota: number
  refund_quota: number
  refund_amount: string
  currency: string
}

export async function previewTopupRefund(tradeNo: string) {
  const res = await api.get<ApiResponse<TopupRefundResult>>(
    `/api/adty/admin/topup/${encodeURIComponent(tradeNo)}/refund-preview`
  )
  return requireServerSuccess(res.data).data
}

export async function refundTopup(
  tradeNo: string,
  reason: string,
  idempotencyKey: string,
  refundQuota: number
) {
  const res = await api.post<ApiResponse<TopupRefundResult>>(
    `/api/adty/admin/topup/${encodeURIComponent(tradeNo)}/refund`,
    { reason, idempotency_key: idempotencyKey, refund_quota: refundQuota }
  )
  return requireServerSuccess(res.data).data
}
