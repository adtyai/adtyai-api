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
 * Bonus percent for an amount: the rate of the largest tier not above it.
 * Rates come from the server (admins can change them); nothing is hard-coded.
 */
export function bonusRateFor(
  amount: number,
  rates: Record<string, number>
): number {
  let bestTier = -1
  let bestRate = 0
  for (const [tier, rate] of Object.entries(rates)) {
    const value = Number(tier)
    if (!Number.isFinite(value) || rate <= 0) continue
    if (value <= amount && value > bestTier) {
      bestTier = value
      bestRate = rate
    }
  }
  return bestRate
}

/** Bonus in USD for display, rounded to cents. */
export function bonusUsdFor(
  amount: number,
  rates: Record<string, number>
): number {
  return Math.round(amount * bonusRateFor(amount, rates)) / 100
}
