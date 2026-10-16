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
  formatBillingCurrencyFromUSD,
  getCurrencyDisplay,
} from '@/lib/currency'

/** Convert aff quota units to USD (quotaPerUnit = 1 USD). */
export function affQuotaToUsd(quota: number | null | undefined): number {
  if (quota == null || Number.isNaN(Number(quota))) return 0
  const { config } = getCurrencyDisplay()
  const per = config.quotaPerUnit > 0 ? config.quotaPerUnit : 500000
  return Number(quota) / per
}

/** Convert USD amount to aff quota units for API submit. */
export function affUsdToQuota(usd: number): number {
  if (!Number.isFinite(usd) || usd <= 0) return 0
  const { config } = getCurrencyDisplay()
  const per = config.quotaPerUnit > 0 ? config.quotaPerUnit : 500000
  return Math.round(usd * per)
}

/** Always show aff amounts as billing USD (never tokens). */
export function formatAffUsd(quota: number | null | undefined): string {
  return formatBillingCurrencyFromUSD(affQuotaToUsd(quota), {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })
}

export function formatAffRate(rate: number | null | undefined): string {
  if (rate == null || Number.isNaN(Number(rate))) return '-'
  const pct = Number(rate) * 100
  const s = pct
    .toFixed(2)
    .replace(/\.0+$/, '')
    .replace(/(\.[1-9]*)0+$/, '$1')
  return `${s}%`
}

export function estimateWithdrawFeeQuota(
  amountQuota: number,
  feeRate: number
): { feeQuota: number; netQuota: number } {
  let feeQuota = Math.floor(amountQuota * feeRate)
  if (feeQuota < 0) feeQuota = 0
  if (feeQuota >= amountQuota) {
    feeQuota = Math.max(0, amountQuota - 1)
  }
  return { feeQuota, netQuota: amountQuota - feeQuota }
}

export function newWithdrawIdemKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID()
  }
  return `aff-wd-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}
