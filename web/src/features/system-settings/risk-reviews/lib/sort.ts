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

import type { RiskReview } from '../types'

/** Pending first, then by id desc (API already id DESC; keep pending on top when mixed). */
export function sortRiskReviewsForDisplay(rows: RiskReview[]): RiskReview[] {
  return [...rows].sort((a, b) => {
    const ap = a.status === 'pending' ? 0 : 1
    const bp = b.status === 'pending' ? 0 : 1
    if (ap !== bp) return ap - bp
    return (b.id || 0) - (a.id || 0)
  })
}

export function formatUsd(n: number | undefined | null): string {
  if (n == null || Number.isNaN(Number(n))) return '-'
  return `$${  Number(n).toFixed(2)}`
}
