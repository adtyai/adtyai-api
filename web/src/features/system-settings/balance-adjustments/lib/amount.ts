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

export function quotaPerUnit(): number {
  const { config } = getCurrencyDisplay()
  return config.quotaPerUnit > 0 ? config.quotaPerUnit : 500000
}

/** Quota (signed int64) to USD for display. */
export function adjustQuotaToUsd(quota: number | null | undefined): number {
  if (quota == null || Number.isNaN(Number(quota))) return 0
  return Number(quota) / quotaPerUnit()
}

/** USD (signed) to quota int for API submit. Zero returns 0. */
export function adjustUsdToQuota(usd: number): number {
  if (!Number.isFinite(usd) || usd === 0) return 0
  return Math.round(usd * quotaPerUnit())
}

export function formatAdjustUsd(quota: number | null | undefined): string {
  return formatBillingCurrencyFromUSD(adjustQuotaToUsd(quota), {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })
}

export function balanceAdjustErrorMessage(
  code: string | undefined,
  fallback: string
): string {
  switch (code) {
    case 'self_approve_forbidden':
      return 'Self-approve forbidden when multiple admins'
    case 'already_decided':
      return 'Request already decided'
    case 'not_found':
      return 'Balance adjust request not found'
    case 'confirm_required':
    case 'bad_request':
      return fallback
    default:
      return fallback
  }
}
