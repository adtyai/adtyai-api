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

/** API discount is 0.1–1 (multiplier). UI may show percent 10–100. */

export function discountToPercent(d: number): number {
  return Math.round(d * 1000) / 10
}

export function percentToDiscount(p: number): number {
  return Math.round(p * 10) / 1000
}

export function formatDiscount(d: number | undefined | null): string {
  if (d == null || Number.isNaN(d)) return '-'
  const pct = discountToPercent(d)
  const dec =
    Math.round(d * 1000) / 1000 === d
      ? String(d)
      : (Math.round(d * 1000) / 1000).toFixed(3)
  return `${pct}% (${dec})`
}

export function isValidDiscount(d: number): boolean {
  return Number.isFinite(d) && d >= 0.1 && d <= 1
}

export function scopeLabel(
  value: string | null | undefined,
  wildcard: string
): string {
  const v = (value ?? '').trim()
  return v || wildcard
}
