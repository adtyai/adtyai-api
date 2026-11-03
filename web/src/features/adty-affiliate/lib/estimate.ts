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
/**
 * Monthly commission estimate: people × average top-up × rate%.
 * Returns null for unusable input so the page shows a dash instead of $0.
 */
export function estimateCommission(
  people: number,
  averageTopUp: number,
  ratePercent: number
): number | null {
  const values = [people, averageTopUp, ratePercent]
  if (values.some((value) => !Number.isFinite(value) || value < 0)) return null
  return Math.round(people * averageTopUp * ratePercent) / 100
}
