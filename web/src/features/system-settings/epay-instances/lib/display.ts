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

import type { EpayInstanceView } from '../types'

export function formatSecretFingerprint(fp: string | undefined | null): string {
  if (!fp) return '-'
  return fp
}

export function formatDailyLimit(n: number | undefined | null): string {
  if (n == null || n === 0) return 'unlimited'
  return String(n)
}

export function primaryInstance(
  rows: EpayInstanceView[]
): EpayInstanceView | null {
  return rows.find((r) => r.is_primary && r.status === 'enabled') || null
}
