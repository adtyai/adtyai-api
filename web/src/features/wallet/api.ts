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

import { getAdtyDeviceFp } from './lib/device-fp'
import type {
  RedemptionRequest,
  PaymentRequest,
  AmountRequest,
  AffiliateTransferRequest,
  ApiResponse,
  TopupInfoResponse,
  RedemptionResponse,
  AmountResponse,
  PaymentResponse,
  StripePaymentResponse,
  AffiliateCodeResponse,
  AffiliateTransferResponse,
  BillingHistoryResponse,
  CompleteOrderRequest,
  CreemPaymentRequest,
  CreemPaymentResponse,
  WaffoPaymentRequest,
  WaffoPaymentResponse,
  WaffoPancakePaymentRequest,
  WaffoPancakePaymentResponse,
  AdtyTopupTiersResponse,
  AdtyFirstRechargeResponse,
  AdtyCreateTopupOrderRequest,
  AdtyCreateTopupOrderResponse,
  AdtySelfTopupOrdersResponse,
  AdtyContinueTopupResponse,
  AdtyTopupOrder,
} from './types'

// ============================================================================
// Wallet API Functions
// ============================================================================

/**
 * Check if API response is successful
 */
export function isApiSuccess(response: ApiResponse): boolean {
  return response.success === true || response.message === 'success'
}

/**
 * Get topup configuration info
 */
export async function getTopupInfo(): Promise<TopupInfoResponse> {
  const res = await api.get('/api/user/topup/info')
  return res.data
}

/**
 * Redeem a topup code
 */
export async function redeemTopupCode(
  request: RedemptionRequest
): Promise<RedemptionResponse> {
  const res = await api.post('/api/user/topup', request)
  return res.data
}

/**
 * Calculate payment amount for regular payment
 */
export async function calculateAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Stripe payment
 */
export async function calculateStripeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/stripe/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Waffo payment
 */
export async function calculateWaffoAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/waffo/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Legacy official top-up path (POST /api/user/pay) — disabled.
 * Live wallet UI uses createAdtyTopupOrder → POST /api/topup/order only.
 */
export async function requestPayment(
  _request: PaymentRequest
): Promise<PaymentResponse> {
  throw new Error(
    'Legacy /api/user/pay is disabled; use POST /api/topup/order via createAdtyTopupOrder'
  )
}

/**
 * Legacy Stripe top-up — closed. Live wallet uses createAdtyTopupOrder.
 */
export async function requestStripePayment(
  _request: PaymentRequest
): Promise<StripePaymentResponse> {
  throw new Error(
    'Legacy Stripe top-up is closed; use POST /api/topup/order via createAdtyTopupOrder'
  )
}

/**
 * Legacy Creem top-up — closed. Live wallet uses createAdtyTopupOrder.
 */
export async function requestCreemPayment(
  _request: CreemPaymentRequest
): Promise<CreemPaymentResponse> {
  throw new Error(
    'Legacy Creem top-up is closed; use POST /api/topup/order via createAdtyTopupOrder'
  )
}

/**
 * Legacy Waffo top-up — closed. Live wallet uses createAdtyTopupOrder.
 */
export async function requestWaffoPayment(
  _request: WaffoPaymentRequest
): Promise<WaffoPaymentResponse> {
  throw new Error(
    'Legacy Waffo top-up is closed; use POST /api/topup/order via createAdtyTopupOrder'
  )
}

/**
 * Calculate payment amount for Waffo Pancake payment
 */
export async function calculateWaffoPancakeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/waffo-pancake/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Legacy Waffo Pancake top-up — closed. Live wallet uses createAdtyTopupOrder.
 */
export async function requestWaffoPancakePayment(
  _request: WaffoPancakePaymentRequest
): Promise<WaffoPancakePaymentResponse> {
  throw new Error(
    'Legacy Waffo Pancake top-up is closed; use POST /api/topup/order via createAdtyTopupOrder'
  )
}

/**
 * Get affiliate code
 */
export async function getAffiliateCode(): Promise<AffiliateCodeResponse> {
  const res = await api.get('/api/user/aff')
  return res.data
}

/**
 * Transfer affiliate quota to balance
 */
export async function transferAffiliateQuota(
  request: AffiliateTransferRequest
): Promise<AffiliateTransferResponse> {
  const res = await api.post('/api/user/aff_transfer', request)
  return res.data
}

/**
 * Get billing history for current user
 */
export async function getUserBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup/self?${params.toString()}`)
  return res.data
}

/**
 * Get billing history for all users (admin only)
 */
export async function getAllBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup?${params.toString()}`)
  return res.data
}

/**
 * Legacy official complete path (POST /api/user/topup/complete) — disabled.
 * Root manual complete uses POST /api/admin/orders/:no/complete on the orders page.
 */
export async function completeOrder(
  _request: CompleteOrderRequest
): Promise<ApiResponse> {
  throw new Error(
    'Legacy /api/user/topup/complete is disabled; use POST /api/admin/orders/:no/complete on system-settings/orders (Root)'
  )
}

// ============================================================================
// ADTY fixed-tier topup APIs
// ============================================================================

/**
 * GET /api/topup/tiers — fixed tiers, first_topup_bonus_rate, pay_methods
 */
export async function getAdtyTopupTiers(): Promise<AdtyTopupTiersResponse> {
  const res = await api.get('/api/topup/tiers', {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * GET /api/topup/first-recharge — whether current user is first-topup eligible
 */
export async function getAdtyFirstRecharge(): Promise<AdtyFirstRechargeResponse> {
  const res = await api.get('/api/topup/first-recharge', {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * POST /api/topup/order — only fixed tier amounts; invalid → 400
 */
export async function createAdtyTopupOrder(
  request: AdtyCreateTopupOrderRequest
): Promise<AdtyCreateTopupOrderResponse> {
  const res = await api.post('/api/topup/order', request, {
    skipBusinessError: true,
    headers: {
      'X-Device-FP': getAdtyDeviceFp(),
    },
  })
  return res.data
}

/**
 * GET /api/topup/order/:no — poll order status (pending | paid | failed | need_manual)
 */
export async function listAdtyTopupOrders(
  page = 1,
  pageSize = 10
): Promise<AdtySelfTopupOrdersResponse> {
  const res = await api.get('/api/topup/orders', {
    params: { page, page_size: pageSize },
    skipErrorHandler: true,
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

export async function continueAdtyTopupOrder(
  orderNo: string
): Promise<AdtyContinueTopupResponse> {
  const res = await api.post(
    `/api/topup/order/${encodeURIComponent(orderNo)}/continue`,
    {},
    {
      skipBusinessError: true,
    }
  )
  return res.data
}

export async function getAdtyTopupOrder(
  orderNo: string
): Promise<ApiResponse<AdtyTopupOrder>> {
  const res = await api.get(`/api/topup/order/${encodeURIComponent(orderNo)}`, {
    skipErrorHandler: true,
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}
