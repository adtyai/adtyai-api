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
import type { StatusVariant } from '@/components/status-badge'

import type {
  ReferralCommissionStatus,
  ReferralProfile,
  ReferralWithdrawalStatus,
  WithdrawalAccountType,
} from '../types'

// Values are i18n keys; render them with t().

export const COMMISSION_STATUS_LABELS: Record<
  ReferralCommissionStatus,
  string
> = {
  pending: 'Settling',
  available: 'Withdrawable',
  frozen: 'Withdrawing',
  paid: 'Withdrawn',
  void: 'Voided',
  clawed_back: 'Clawed back',
}

export const COMMISSION_STATUS_VARIANTS: Record<
  ReferralCommissionStatus,
  StatusVariant
> = {
  pending: 'warning',
  available: 'success',
  frozen: 'info',
  paid: 'neutral',
  void: 'neutral',
  clawed_back: 'danger',
}

export const WITHDRAWAL_STATUS_LABELS: Record<
  ReferralWithdrawalStatus,
  string
> = {
  pending: 'Under review',
  approved: 'Approved, awaiting payout',
  paid: 'Paid',
  rejected: 'Rejected',
  canceled: 'Canceled',
}

export const WITHDRAWAL_STATUS_VARIANTS: Record<
  ReferralWithdrawalStatus,
  StatusVariant
> = {
  pending: 'warning',
  approved: 'info',
  paid: 'success',
  rejected: 'danger',
  canceled: 'neutral',
}

export const WITHDRAWAL_TARGET_LABELS: Record<WithdrawalAccountType, string> = {
  balance: 'Account balance',
  usdt: 'USDT',
  alipay: 'Alipay',
  wechat: 'WeChat Pay',
}

/** A new idempotency key for one withdrawal attempt. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

/** Which targets the admin has configured (adty convention I: no built-in minimum, fee or rate). */
export function withdrawTargetAvailable(
  profile: ReferralProfile,
  target: WithdrawalAccountType
): boolean {
  if (target === 'balance') return true
  if (profile.min_withdraw_usd === null) return false
  if (target === 'usdt') return profile.usdt_withdraw_fee !== null
  return profile.cny_rate !== ''
}
