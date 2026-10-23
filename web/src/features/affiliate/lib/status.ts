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
import type { AffCommissionStatus, AffWithdrawStatus } from '../types'

export function commissionStatusLabel(status: AffCommissionStatus): string {
  switch (status) {
    case 'frozen':
      return 'Frozen'
    case 'available':
      return 'Available'
    case 'voided':
      return 'Voided'
    case 'blocked':
      return 'Blocked'
    default:
      return String(status || '-')
  }
}

export function commissionStatusLabels(status: string): string[] {
  const parts = status
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
  if (parts.length === 0) return ['-']
  return parts.map((part) => commissionStatusLabel(part as AffCommissionStatus))
}

export function withdrawMethodLabel(row: {
  pay_method?: string
  usdt_chain?: string
}): string {
  const method = (row.pay_method || '').trim()
  if (method === 'usdt') return 'USDT'
  if (method === 'alipay') return 'Alipay'
  if ((row.usdt_chain || '').trim()) return 'USDT'
  return 'Legacy payout account'
}

export function withdrawStatusLabel(status: AffWithdrawStatus): string {
  switch (status) {
    case 'pending':
      return 'Pending review'
    case 'approved':
      return 'Approved'
    case 'rejected':
      return 'Rejected'
    case 'paid':
      return 'Paid'
    case 'pay_failed':
      return 'Pay failed'
    case 'pay_failed_final':
      return 'Pay failed (final)'
    case 'pay_unknown':
      return 'Pay unknown'
    default:
      return String(status || '-')
  }
}

export type AffAdminWithdrawAction =
  | 'approve'
  | 'reject'
  | 'mark-paid'
  | 'mark-failed'
  | 'mark-unknown'
  | 'retry'

export function adminActionsForWithdrawStatus(
  status: AffWithdrawStatus
): AffAdminWithdrawAction[] {
  switch (status) {
    case 'pending':
      return ['approve', 'reject']
    case 'approved':
      return ['mark-paid', 'mark-failed', 'mark-unknown']
    case 'pay_failed':
      return ['retry']
    case 'pay_unknown':
      return ['mark-paid', 'mark-failed']
    case 'pay_failed_final':
    case 'paid':
    case 'rejected':
    default:
      return []
  }
}

export function canRetryWithdraw(status: AffWithdrawStatus): boolean {
  return status === 'pay_failed'
}
