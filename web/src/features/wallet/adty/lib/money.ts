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
// formatNumber has no fixed-fraction option; USD amounts always show 2 decimals.
/** Face plus tier bonus plus promo bonus, the dollars the user is told they receive. */
export function creditedUsdAmount(ext: {
  face_usd: string
  bonus_usd: string
  promo_bonus_usd?: string
}): string {
  const total =
    Number(ext.face_usd) +
    Number(ext.bonus_usd) +
    Number(ext.promo_bonus_usd || 0)
  return total.toFixed(2)
}

export function formatUsd(value: number, locale: string | undefined): string {
  return `$${new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)}`
}

export function formatPaymentAmount(
  amount: string,
  currency: string,
  locale: string | undefined
): string {
  if (currency === 'USDT') return `${amount} USDT`
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency,
    }).format(Number(amount))
  } catch {
    return `${amount} ${currency}`
  }
}
