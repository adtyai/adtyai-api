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
// Referral types, USD only. Money settings are null until the admin sets them (adty convention I).

export type ReferralAffiliate = {
  id: number
  user_id: number
  invite_code: string
  status: 'approved' | 'disabled'
  partner_status: '' | 'pending' | 'approved' | 'rejected'
  applicant_note: string
  rate_override: number | null
  acquisition_enabled: boolean
  settlement_enabled: boolean
  withdrawal_enabled: boolean
  risk_reason: string
  risk_note: string
  site_owner_since: number
  created_at: number
}

export type ReferralAccount = {
  pending_amount: number
  available_amount: number
  frozen_amount: number
  withdrawn_amount: number
}

export type ReferralProfile = {
  affiliate: ReferralAffiliate
  account: ReferralAccount
  /** Percent, "10" means 10%. Empty when no rate is set. */
  rate: string
  freeze_days: number
  min_withdraw_usd: number | null
  usdt_withdraw_fee: number | null
  cancel_window_minutes: number
  /** Empty when the CNY withdrawal rate is not set. */
  cny_rate: string
  cny_rate_updated_at: number
  enabled: boolean
  site_owner: boolean
  funnel: { clicks: number; signups: number; paying: number; earned: string }
}

export type ReferralCommissionStatus =
  | 'pending'
  | 'available'
  | 'frozen'
  | 'paid'
  | 'void'
  | 'clawed_back'

export type ReferralCommission = {
  id: number
  affiliate_id: number
  invitee_user_id: number
  source_trade_no: string
  base_amount: number
  rate: number
  commission_amount: number
  status: ReferralCommissionStatus
  settle_at: number
  created_at: number
}

export type WithdrawalAccountType = 'balance' | 'usdt' | 'alipay' | 'wechat'

export type ReferralWithdrawalStatus =
  | 'pending'
  | 'approved'
  | 'paid'
  | 'rejected'
  | 'canceled'

export type ReferralWithdrawal = {
  id: number
  affiliate_id: number
  user_id: number
  amount: number
  fee_amount: number
  net_amount: number
  account_type: WithdrawalAccountType
  account_name: string
  account_no: string
  account_network: string
  qr_image_url: string
  cny_rate: string
  cny_amount: string
  admin_note: string
  payment_txn_no: string
  payment_proof_url: string
  status: ReferralWithdrawalStatus
  reject_reason: string
  submitted_at: number
  paid_at: number
}

export type WithdrawalRequest = {
  amount: string
  account_type: WithdrawalAccountType
  account_network?: string
  account_no?: string
  account_name?: string
  qr_image_url?: string
  expected_cny_rate?: string
  idempotency_key: string
}

export type ReferralPage<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type ReferralPublicSettings = {
  enabled: boolean
  default_rate: number | null
  freeze_days: number
  min_withdraw_usd: number | null
  usdt_withdraw_fee: number | null
  cookie_ttl_days: number
}
