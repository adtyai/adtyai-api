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
export type AffCommissionStatus =
  | 'frozen'
  | 'available'
  | 'voided'
  | 'blocked'
  | string

export type AffWithdrawStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'paid'
  | 'pay_failed'
  | 'pay_failed_final'
  | 'pay_unknown'
  | string

export type AffSelfSummary = {
  aff_code: string
  aff_count: number
  aff_quota: number
  aff_history: number
  frozen_quota: number
  inviter_id: number
  min_withdraw_usd: number
  withdraw_fee_rate: number
  freeze_days: number
  topup_rate: number
  subscription_rate?: number
}

export type AffCommission = {
  id: number
  inviter_id: number
  invitee_id: number
  inviter_name?: string
  invitee_name?: string
  order_no: string
  source: string
  paid_amount_usd: number
  rate_snapshot: number
  commission_quota: number
  freeze_days_snapshot: number
  status: AffCommissionStatus
  unfreeze_at: string
  block_reason?: string
  voided_by?: number
  voided_at?: string | null
  created_at: string
  updated_at: string
}

export type AffWithdrawal = {
  id: number
  user_id: number
  username?: string
  amount_quota: number
  fee_quota: number
  net_quota: number
  fee_rate_snapshot: number
  min_amount_snapshot: number
  idem_key: string
  status: AffWithdrawStatus
  pay_method?: string
  pay_account: string
  usdt_chain?: string
  payout_ref?: string
  payout_qr_id?: number
  reject_reason?: string
  operator_id?: number
  created_at: string
  updated_at: string
  resolved_at?: string | null
}

export type AffListResponse<T> = {
  success: boolean
  message?: string
  code?: string
  data?: T[]
  total?: number
  counts?: Record<string, number>
}

export type AffOneResponse<T> = {
  success: boolean
  message?: string
  code?: string
  data?: T
}
