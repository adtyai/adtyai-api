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
  EXCLUDED_GROUPS,
  QUOTA_TYPE_VALUES,
} from '@/features/pricing/constants'
import {
  getSavingsRatio,
  parseOfficialPrice,
  type OfficialPrices,
} from '@/features/pricing/lib/adty-savings'
import { isDynamicPricingModel } from '@/features/pricing/lib/dynamic-price'
import { getDisplayGroupRatio } from '@/features/pricing/lib/model-helpers'
import { calculateTokenPrice } from '@/features/pricing/lib/price'
import type { PricingModel } from '@/features/pricing/types'

/** USD per 1M tokens (input / output), or USD per call. */
export type LandingPrice =
  | { kind: 'tokens'; input: number; output: number }
  | { kind: 'per_call'; value: number }

/** One row of the home page price table (adty task 8.3). */
export type LandingPriceRow = {
  model: string
  vendor: string
  official: LandingPrice
  site: LandingPrice
  /** 1 − site ÷ official, from the pricing page's savings rule; may be ≤ 0. */
  savings: number
}

/**
 * Group the prices are shown for, the same choice as the pricing page: the
 * viewer's own group, else `default`, else undefined (cheapest usable group).
 */
export function getLandingGroup(
  userGroup: string | undefined,
  usableGroups: Record<string, unknown>
): string | undefined {
  const groups = Object.keys(usableGroups).filter(
    (group) => !EXCLUDED_GROUPS.includes(group)
  )
  const preferred = userGroup || 'default'
  return groups.includes(preferred) ? preferred : undefined
}

/**
 * Table row for a model, or null when the model has no official price of the
 * same kind. Dynamic (expression) pricing has no single price to compare and
 * stays on the pricing page.
 */
export function toLandingPriceRow(
  model: PricingModel,
  officialPrices: OfficialPrices,
  group: string | undefined
): LandingPriceRow | null {
  const official = officialPrices[model.model_name]
  if (!official || isDynamicPricingModel(model)) return null
  const savings = getSavingsRatio(model, official, group)
  if (savings == null) return null
  const ratio = getDisplayGroupRatio(model, group)
  const vendor = model.vendor_name ?? ''

  if (model.quota_type === QUOTA_TYPE_VALUES.REQUEST) {
    const officialPerCall = parseOfficialPrice(official.per_call)
    if (officialPerCall == null || typeof model.model_price !== 'number') {
      return null
    }
    return {
      model: model.model_name,
      vendor,
      official: { kind: 'per_call', value: officialPerCall },
      site: { kind: 'per_call', value: model.model_price * ratio },
      savings,
    }
  }

  const officialInput = parseOfficialPrice(official.input)
  const officialOutput = parseOfficialPrice(official.output)
  if (officialInput == null || officialOutput == null) return null
  return {
    model: model.model_name,
    vendor,
    official: { kind: 'tokens', input: officialInput, output: officialOutput },
    site: {
      kind: 'tokens',
      input: calculateTokenPrice(model, 'input', ratio),
      output: calculateTokenPrice(model, 'output', ratio),
    },
    savings,
  }
}

/** Rows for every model that can be compared, in the order of /api/pricing. */
export function getLandingPriceRows(
  models: PricingModel[],
  officialPrices: OfficialPrices,
  group: string | undefined
): LandingPriceRow[] {
  return models.flatMap((model) => {
    const row = toLandingPriceRow(model, officialPrices, group)
    return row ? [row] : []
  })
}

/**
 * The rows shown on the home page: a few per vendor in turn, so one vendor
 * with many models does not fill the table. Inside a vendor the biggest
 * savings come first.
 */
export function pickPopularRows(
  rows: LandingPriceRow[],
  limit: number
): LandingPriceRow[] {
  const byVendor = new Map<string, LandingPriceRow[]>()
  for (const row of rows) {
    const list = byVendor.get(row.vendor) ?? []
    list.push(row)
    byVendor.set(row.vendor, list)
  }
  const queues = [...byVendor.values()].map((list) =>
    [...list].sort((a, b) => b.savings - a.savings)
  )
  const picked: LandingPriceRow[] = []
  for (let round = 0; picked.length < limit; round++) {
    const next = queues.filter((queue) => round < queue.length)
    if (next.length === 0) break
    for (const queue of next) {
      if (picked.length >= limit) break
      picked.push(queue[round])
    }
  }
  return picked
}

/**
 * The row with the largest positive savings ("up to X% off" and the hero
 * price card); null when no model is cheaper than its official price.
 */
export function getBestSavingsRow(
  rows: LandingPriceRow[]
): LandingPriceRow | null {
  let best: LandingPriceRow | null = null
  for (const row of rows) {
    if (row.savings > 0 && (best == null || row.savings > best.savings)) {
      best = row
    }
  }
  return best
}
