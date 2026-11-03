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
import { QUOTA_TYPE_VALUES } from '../constants'
import type { PricingModel } from '../types'
import { getDynamicPricingSummary } from './dynamic-price'
import { getDisplayGroupRatio } from './model-helpers'
import { calculateTokenPrice } from './price'

/**
 * Official list price of a model (adty task 8.5), USD per 1M tokens or per
 * call. The server sends decimals as strings and missing prices as null.
 */
export type OfficialPrice = {
  input: string | null
  output: string | null
  cache_read: string | null
  cache_write: string | null
  per_call: string | null
}

export type OfficialPrices = Record<string, OfficialPrice>

export function parseOfficialPrice(value: string | null): number | null {
  if (value == null || value === '') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

/**
 * Site price in USD that is compared with the official price: the input
 * price per 1M tokens, or the per-call price for per-request models. Uses the
 * same group ratio as the price cell and ignores the recharge display mode.
 */
function getSiteComparablePrice(
  model: PricingModel,
  selectedGroup?: string
): { kind: 'input' | 'per_call'; value: number } | null {
  const groupRatio = getDisplayGroupRatio(model, selectedGroup)
  const summary = getDynamicPricingSummary(model, { tokenUnit: 'M' })
  if (summary) {
    const perCall = summary.entries.find(
      (entry) => entry.field === 'fixedPrice' || entry.field === 'modelPrice'
    )
    if (perCall) return { kind: 'per_call', value: perCall.value * groupRatio }
    const input = summary.entries.find(
      (entry) => entry.field === 'inputPrice' && entry.unit === 'token'
    )
    return input ? { kind: 'input', value: input.value * groupRatio } : null
  }
  if (model.quota_type === QUOTA_TYPE_VALUES.REQUEST) {
    return typeof model.model_price === 'number'
      ? { kind: 'per_call', value: model.model_price * groupRatio }
      : null
  }
  return {
    kind: 'input',
    value: calculateTokenPrice(model, 'input', groupRatio),
  }
}

/**
 * Savings = 1 − site price ÷ official price. Null when the model has no
 * official price of the compared kind. May be negative.
 */
export function getSavingsRatio(
  model: PricingModel,
  official: OfficialPrice | undefined,
  selectedGroup?: string
): number | null {
  if (!official) return null
  const site = getSiteComparablePrice(model, selectedGroup)
  if (!site || !Number.isFinite(site.value)) return null
  const officialValue = parseOfficialPrice(
    site.kind === 'per_call' ? official.per_call : official.input
  )
  if (officialValue == null || officialValue <= 0) return null
  return 1 - site.value / officialValue
}

/** Whole-percent savings for display, rounded down so it never overstates. */
export function formatSavingsPercent(ratio: number, locale?: string): string {
  // Round first to absorb floating-point noise such as 0.7999999.
  const percent = Math.floor(Math.round(ratio * 1e6) / 1e4)
  return new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 0,
  }).format(percent / 100)
}
