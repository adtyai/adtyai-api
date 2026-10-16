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

export const RECONCILE_STATUSES = [
  'ok',
  'incomplete',
  'mismatch',
  'failed',
] as const

export type ReconcileStatus = (typeof RECONCILE_STATUSES)[number]

export type ReconcileRefundCheck = {
  registered_quota: number
  paid_clawed: number
  equal: boolean
  bonus_clawed: number
  aff_clawed: number
  bonus_shortfall: number
}

export type ReconcileResult = {
  date: string
  timezone: string
  status: string
  missing_buckets: string[]
  backfill_status: string
  last_backup: string | null
  balanced: boolean
  diff?: number
  ledger_sum?: number
  unrolled_consume?: number
  wallet_paid_sum?: number
  wallet_bonus_sum?: number
  refund: ReconcileRefundCheck
  order_nos?: string[]
  mismatch_users?: number
  mismatch_user_ids?: number[]
  /** 按账户核对起始日；空/缺失 = 未开启 */
  account_check_from?: string
  account_checked_users?: number
  /** 按账户对不上的用户数，只报告，不计入 balanced */
  account_drift_users?: number
  account_drift_user_ids?: number[]
  skipped?: boolean
  note?: string
}

export type ReconcileGetResponse = {
  success: boolean
  message?: string
  data?: ReconcileResult
}
