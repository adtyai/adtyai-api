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

export type BalanceAdjustStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'executed'
  | string

export type BalanceAdjustAccount = 'quota' | 'bonus' | string

export type BalanceAdjustRequest = {
  id: number
  user_id: number
  account: BalanceAdjustAccount
  amount: number
  reason: string
  chatwoot_conversation_id?: string
  status: BalanceAdjustStatus
  applicant_id: number
  approver_id?: number | null
  reject_reason?: string
  self_approve_reason?: string
  ledger_entry_id?: number | null
  created_at?: string
  decided_at?: string | null
  executed_at?: string | null
}

export type ApproveBalanceAdjustResult = {
  request: BalanceAdjustRequest
  balance_before: number
  balance_after: number
  ledger_entry_id: number
  self_approved: boolean
  admin_count: number
}

export type BalanceAdjustListResponse = {
  success: boolean
  message?: string
  code?: string
  data?: BalanceAdjustRequest[]
  total?: number
}

export type BalanceAdjustOneResponse = {
  success: boolean
  message?: string
  code?: string
  data?: BalanceAdjustRequest
}

export type BalanceAdjustApproveResponse = {
  success: boolean
  message?: string
  code?: string
  data?: ApproveBalanceAdjustResult
}
