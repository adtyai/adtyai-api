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

export type TopupRefundStatus = 'none' | 'partial' | 'full' | string

export type TopupOrder = {
  id: number
  order_no: string
  user_id: number
  amount_usd: number
  paid_amount_usd: number
  credit_amount_usd: number
  bonus_amount_usd: number
  pending_extra_usd?: number
  status: string
  payment_method?: string
  payment_provider?: string
  refund_status: TopupRefundStatus
  refunded_amount: number
  create_time?: number
  complete_time?: number
  paid_credited_quota?: number
  bonus_credited_quota?: number
}

export type TopupRefundRow = {
  id: number
  order_no: string
  request_id: string
  amount: number
  reason: string
  operator_id: number
  bonus_clawed: number
  bonus_shortfall: number
  aff_clawed: number
  ledger_entry_ids?: string
  created_at?: string
}

export type RefundTopupOrderResult = {
  refund?: TopupRefundRow
  order: TopupOrder
  paid_clawed: number
  bonus_clawed: number
  bonus_shortfall: number
  aff_clawed: number
  paid_before: number
  paid_after: number
  bonus_before: number
  bonus_after: number
  aff_before: number
  aff_after: number
  ledger_entry_ids?: number[]
  refundable_before: number
}

export type OrderOneResponse = {
  success: boolean
  message?: string
  code?: string
  data?: TopupOrder
}

export type RefundOneResponse = {
  success: boolean
  message?: string
  code?: string
  data?: RefundTopupOrderResult
}
