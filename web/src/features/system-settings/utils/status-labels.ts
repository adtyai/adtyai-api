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
/** i18n label keys for raw status / kind codes shown in admin pages. Unknown codes fall back to the raw value. */
export function riskReviewStatusLabelKey(status: string): string {
  switch (status) {
    case 'pending':
      return 'Pending'
    case 'approved':
      return 'Approved'
    case 'rejected':
      return 'Rejected'
    default:
      return status || '-'
  }
}

export function riskReviewKindLabelKey(kind: string): string {
  switch (kind) {
    case 'first_topup_extra':
      return 'First top-up bonus'
    case 'aff_self_invite':
      return 'Affiliate self-invite'
    default:
      return kind || '-'
  }
}

export function commissionSourceLabelKey(source: string): string {
  switch (source) {
    case 'topup':
      return 'Top-up'
    case 'subscription':
      return 'Subscription'
    default:
      return source || '-'
  }
}

/**
 * Risk-control reason codes (model.DetectAffSelfInvite, topup_tiers
 * PendingReason) shown on risk reviews / withdrawals. Returns null for unknown
 * or free-text reasons so callers render them unchanged.
 */
const riskReasonLabelKeys: Readonly<Record<string, string>> = {
  same_device_fp: 'Same device fingerprint',
  same_ip: 'Same IP address',
  same_email: 'Same email address',
  missing_device_fp: 'Missing device fingerprint',
  ip_conflict: 'IP conflict',
}

export function riskReasonLabelKey(reason: string): string | null {
  return Object.hasOwn(riskReasonLabelKeys, reason)
    ? riskReasonLabelKeys[reason]
    : null
}

/**
 * BackupContactType enum (model/business_params.go: email | telegram) → i18n
 * key; unknown → null so the raw value is shown.
 */
export function backupContactTypeLabelKey(type: string): string | null {
  switch (type) {
    case 'email':
      return 'Email'
    case 'telegram':
      return 'Telegram'
    default:
      return null
  }
}
