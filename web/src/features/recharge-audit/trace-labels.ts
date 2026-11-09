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
import type { OrderTraceSection } from './api'

export const ORDER_TRACE_SECTIONS: Record<OrderTraceSection, string> = {
  payments: 'Payment events and queries',
  wallet: 'Wallet credits, adjustments and refunds',
  bonuses: 'Promo bonus records',
  commissions: 'Commission records',
  commission_movements: 'Commission wallet movements',
  site_wallet: 'Site wallet movements',
  exceptions: 'Payment reconciliation events',
  reviews: 'Financial review history',
}

export const ORDER_TRACE_LABELS: Record<string, string> = {
  callback: 'Payment callback',
  query: 'Payment query',
  manual: 'Manual completion',
  credited: 'Credited',
  replayed: 'Payment replay',
  unpaid: 'Not paid',
  failed: 'Failed',
  rejected: 'Rejected',
  received: 'Received',
  paid: 'Paid',
  void: 'Voided',
  pending: 'Pending',
  success: 'Success',
  refunded: 'Refunded',
  expired: 'Expired',
  canceled: 'Canceled',
  replaced: 'Replaced',
  topup: 'Top-up',
  refund: 'Refund',
  manual_adjust: 'Balance adjustment',
  pending_review: 'Pending review',
  never_credited: 'Never credited',
  dismissed: 'Dismissed',
  available: 'Available',
  frozen: 'Frozen',
  settled: 'Settled',
  voided: 'Voided',
  clawed_back: 'Clawed back',
  commission_accrue: 'Commission accrued',
  commission_settle: 'Commission settled',
  commission_void: 'Commission voided',
  commission_clawback: 'Commission clawback',
  commission_clawback_partial: 'Partial commission clawback',
  payment_query: 'Payment query',
  duplicate_payment: 'Duplicate payment',
  operating: 'Operating wallet',
  deposit: 'Deposit wallet',
  earnings: 'Earnings wallet',
}
