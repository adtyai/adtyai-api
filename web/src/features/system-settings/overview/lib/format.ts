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

import { formatQuota } from '@/lib/format'

/** USD paid amount → `$12,345.67`; nullish / NaN → em dash. */
export function formatOverviewUsd(n: number | null | undefined): string {
  if (n == null || Number.isNaN(Number(n))) return '—'
  return (
    '$' +
    Number(n).toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
  )
}

/** Integer count (orders / pending withdrawals) with thousands separators; nullish / NaN → em dash. */
export function formatOverviewCount(n: number | null | undefined): string {
  if (n == null || Number.isNaN(Number(n))) return '—'
  return Math.trunc(Number(n)).toLocaleString('en-US')
}

/** Consume quota; nullish / NaN → em dash (aligned with USD empty display). */
export function formatOverviewQuota(n: number | null | undefined): string {
  if (n == null || Number.isNaN(Number(n))) return '—'
  return formatQuota(Number(n))
}

/**
 * Format as_of (RFC3339) in the given IANA timezone (default Asia/Shanghai).
 * Falls back to the raw string when parsing fails.
 */
export function formatOverviewAsOf(
  asOf: string | null | undefined,
  timezone = 'Asia/Shanghai'
): string {
  if (asOf == null || asOf === '') return '—'
  const d = new Date(asOf)
  if (Number.isNaN(d.getTime())) return asOf
  try {
    return new Intl.DateTimeFormat('zh-CN', {
      timeZone: timezone || 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(d)
  } catch {
    return asOf
  }
}

/** YYYY-MM-DD → M/D for short day label; invalid → original. */
export function formatOverviewDateShort(date: string | null | undefined): string {
  if (!date) return '—'
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date.trim())
  if (!m) return date
  return `${Number(m[2])}/${Number(m[3])}`
}
