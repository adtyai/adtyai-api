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

export const TOPUP_ORDER_STATUSES = ['pending', 'paid', 'failed'] as const

export type TopupOrderStatus = (typeof TOPUP_ORDER_STATUSES)[number] | string

export type TopupRefundStatus = 'none' | 'partial' | 'full' | string

/** Admin topup order — mirrors model.TopupOrder JSON (snake_case). */
export type TopupOrder = {
  id: number
  order_no: string
  user_id: number
  username?: string
  amount_usd: number
  paid_amount_usd: number
  credit_amount_usd: number
  bonus_amount_usd: number
  pending_extra_usd?: number
  tier_index?: number
  tier_bonus_rate?: number
  first_bonus_rate?: number
  is_first_candidate?: boolean
  apply_first_immediately?: boolean
  pending_reason?: string
  device_fp?: string
  client_ip?: string
  payment_method?: string
  payment_provider?: string
  epay_instance_id?: number
  fx_rate?: number
  pay_cny?: number
  pay_usdt?: number
  usdt_fx_rate?: number
  usdt_expire_at?: number
  bonus_rate_snapshot?: number
  provider_trade_no?: string
  status: TopupOrderStatus
  paid_credited_quota?: number
  bonus_credited_quota?: number
  refund_status: TopupRefundStatus
  refunded_amount: number
  create_time?: number
  complete_time?: number
}

export type AdminOrdersListParams = {
  status?: string
  user_id?: number
  payment_method?: string
  payment_provider?: string
  order_no?: string
  create_time_from?: number
  create_time_to?: number
  limit?: number
  offset?: number
}

export type AdminOrdersListResponse = {
  success: boolean
  message?: string
  data?: TopupOrder[]
  total?: number
}

export type AdminOrderDetailResponse = {
  success: boolean
  message?: string
  data?: TopupOrder
}

export type AdminOrderCompleteResponse = {
  success: boolean
  message?: string
  code?: string
  data?: TopupOrder
}
