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
import type {
  AffCommission,
  AffWithdrawal,
} from '@/features/affiliate/types'

export type {
  AffCommission,
  AffCommissionStatus,
  AffListResponse,
  AffOneResponse,
  AffWithdrawStatus,
  AffWithdrawal,
} from '@/features/affiliate/types'

export type { AffAdminWithdrawAction } from '@/features/affiliate/lib/status'

export type AffInviterRow = {
  user_id: number
  username: string
  display_name: string
  aff_code: string
  invitee_count: number
  aff_quota: number
  aff_history: number
}

export type AffOverview = {
  invited_users: number
  inviters: number
  commission_frozen: number
  commission_available: number
  commission_voided: number
  commission_blocked: number
  withdraw_pending_usdt: number
  withdraw_pending_alipay: number
  withdraw_pending_legacy: number
  withdraw_paid_quota: number
}

export type AffAdminUserDetail = {
  user_id: number
  username: string
  display_name: string
  inviter_id: number
  inviter_name: string
  aff_count: number
  aff_quota: number
  aff_history: number
  invitees: {
    user_id: number
    name: string
    registered_at: number
    paid_usd: number
    commission_quota: number
    status: string
  }[]
  invitee_total: number
  commissions: AffCommission[]
  commission_total: number
  withdrawals: AffWithdrawal[]
  withdrawal_total: number
  transfers: {
    id: number
    delta: number
    balance_after: number
    created_at: string
  }[]
  transfer_total: number
}

export type AffRiskReview = {
  id: number
  kind: string
  user_id: number
  order_no: string
  amount_usd: number
  status: string
  reason: string
  payload: string
  resolved_by: number
  created_at: string
  resolved_at?: string | null
  inviter_id?: number
  inviter_name?: string
  invitee_id?: number
  invitee_name?: string
  commission_quota?: number
  commission_status?: string
}
