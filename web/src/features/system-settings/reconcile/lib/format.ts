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

import {
  RECONCILE_STATUSES,
  type ReconcileResult,
  type ReconcileStatus,
} from '../types'

/** Yesterday YYYY-MM-DD in Asia/Shanghai (UTC+8, no DST). */
export function defaultReconcileDate(now: Date = new Date()): string {
  const SHANGHAI_OFFSET_MS = 8 * 60 * 60 * 1000
  const shifted = new Date(now.getTime() + SHANGHAI_OFFSET_MS)
  const y = shifted.getUTCFullYear()
  const m = shifted.getUTCMonth()
  const d = shifted.getUTCDate()
  const yesterday = new Date(Date.UTC(y, m, d))
  yesterday.setUTCDate(yesterday.getUTCDate() - 1)
  const yy = yesterday.getUTCFullYear()
  const mm = String(yesterday.getUTCMonth() + 1).padStart(2, '0')
  const dd = String(yesterday.getUTCDate()).padStart(2, '0')
  return `${yy}-${mm}-${dd}`
}

export function isReconcileStatus(status: string): status is ReconcileStatus {
  return (RECONCILE_STATUSES as readonly string[]).includes(status)
}

/** Stable English keys for t(); keep in sync with locale files. */
export function reconcileStatusLabelKey(status: string): string {
  switch (status) {
    case 'ok':
      return 'Reconcile OK'
    case 'incomplete':
      return 'Reconcile incomplete'
    case 'mismatch':
      return 'Reconcile mismatch'
    case 'failed':
      return 'Reconcile failed'
    default:
      return status || 'Unknown'
  }
}

export function reconcileStatusVariant(
  status: string
): 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'ok':
      return 'secondary'
    case 'mismatch':
    case 'failed':
      return 'destructive'
    case 'incomplete':
    default:
      return 'outline'
  }
}

/** incomplete: only emphasize missing_buckets; no full result panel. */
export function showIncompleteOnly(status: string): boolean {
  return status === 'incomplete'
}

export function showMismatchDetails(status: string): boolean {
  return status === 'mismatch'
}

export function showOkSummary(status: string): boolean {
  return status === 'ok'
}

export function showFailedNote(status: string): boolean {
  return status === 'failed'
}

export function hasMissingBuckets(
  buckets: string[] | null | undefined
): boolean {
  return Array.isArray(buckets) && buckets.length > 0
}

/** Empty buckets → empty string (UI shows "none" / em dash). */
export function formatMissingBuckets(
  buckets: string[] | null | undefined
): string {
  if (!buckets || buckets.length === 0) return ''
  return buckets.join(', ')
}

export function formatLastBackup(
  value: string | null | undefined
): 'none' | string {
  if (value == null || value === '') return 'none'
  return value
}

export function formatQuota(n: number | null | undefined): string {
  if (n == null || Number.isNaN(n)) return '—'
  return String(n)
}

/** Backfill status codes (model.Backfill*) → i18n key; unknown → raw value. */
export function backfillStatusLabelKey(status: string | null | undefined): string {
  switch (status) {
    case 'none':
      return 'Backfill not needed'
    case 'ran':
      return 'Backfill ran'
    case 'partial':
      return 'Backfill partial'
    case 'ok':
      return 'Backfill OK'
    default:
      return status || '—'
  }
}

/**
 * 账户划分偏差用户数（B-03 按账户核对，只报告）。
 * 未设起始日（account_check_from 为空/缺失）→ null，界面显示「未开启」。
 */
export function formatAccountDriftUsers(
  data: Pick<ReconcileResult, 'account_check_from' | 'account_drift_users'>
): string | null {
  if (!data.account_check_from) return null
  return formatQuota(data.account_drift_users ?? 0)
}
