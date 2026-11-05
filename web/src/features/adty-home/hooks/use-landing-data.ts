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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { useAdtySite } from '@/features/adty-site/api'
import { useOfficialPrices } from '@/features/pricing/hooks/use-official-prices'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import {
  getReferralPublicSettings,
  referralQueryKeys,
} from '@/features/referral/api'
import { useStatus } from '@/hooks/use-status'
import { parseHeaderNavModulesFromStatus } from '@/lib/nav-modules'

import {
  getBestSavingsRow,
  getLandingGroup,
  getLandingPriceRows,
  pickPopularRows,
  type LandingPriceRow,
} from '../lib/landing-data'

/** Rows in the home page price table. */
export const POPULAR_MODEL_LIMIT = 6

/**
 * Prices for the landing page. `loading` also covers the build-time render,
 * which has no data. `unavailable` says why the viewer sees no prices: the
 * pricing page needs sign-in, is switched off, or the request failed.
 */
export type LandingPrices =
  | { state: 'loading' }
  | { state: 'unavailable'; reason: 'sign-in' | 'disabled' | 'error' }
  | {
      state: 'ready'
      modelCount: number
      /** Model with the largest positive savings, null when none is cheaper. */
      best: LandingPriceRow | null
      rows: LandingPriceRow[]
    }

export type LandingData = {
  prices: LandingPrices
  /** Minimum top-up in USD; undefined while loading, null when unknown. */
  minTopUp: number | null | undefined
  /** Support hours from the site settings, empty when not set. */
  supportHours: string
  affiliate: {
    /** True once the program is known to be on; otherwise the banner is hidden. */
    open: boolean
    /** Default commission in percent, null when not set (no number shown). */
    rate: number | null
  }
  /** Enabled payment method ids. Omitted until status loads; empty means none. */
  paymentMethodIds?: string[]
}

/** Everything the landing page shows that comes from the server (adty task 8.3). */
export function useLandingData(options: {
  signedIn: boolean
  userGroup?: string
}): LandingData {
  const { status } = useStatus()
  const site = useAdtySite()
  const pricingModule = status
    ? parseHeaderNavModulesFromStatus(status as Record<string, unknown>).pricing
    : null
  // Only ask for prices the viewer may see: a disabled pricing page answers
  // 403 and the shared pricing query would show an error toast.
  const pricingVisible = Boolean(
    pricingModule?.enabled && (!pricingModule.requireAuth || options.signedIn)
  )
  const pricing = usePricingData(pricingVisible)
  const official = useOfficialPrices()
  const referral = useQuery({
    queryKey: referralQueryKeys.publicSettings,
    queryFn: getReferralPublicSettings,
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })

  const prices = useMemo<LandingPrices>(() => {
    if (!pricingModule) return { state: 'loading' }
    if (!pricingModule.enabled) {
      return { state: 'unavailable', reason: 'disabled' }
    }
    if (!pricingVisible) return { state: 'unavailable', reason: 'sign-in' }
    if (pricing.error) return { state: 'unavailable', reason: 'error' }
    if (pricing.isLoading || official.isLoading) return { state: 'loading' }
    const group = getLandingGroup(options.userGroup, pricing.usableGroup)
    const rows = getLandingPriceRows(pricing.models, official.prices, group)
    return {
      state: 'ready',
      modelCount: pricing.models.length,
      best: getBestSavingsRow(rows),
      rows: pickPopularRows(rows, POPULAR_MODEL_LIMIT),
    }
  }, [
    pricingModule,
    pricingVisible,
    pricing.error,
    pricing.isLoading,
    pricing.models,
    pricing.usableGroup,
    official.isLoading,
    official.prices,
    options.userGroup,
  ])

  const referralSettings = referral.data
  return {
    prices,
    minTopUp: site.isPending ? undefined : (site.data?.min_topup ?? null),
    supportHours: site.data?.support_hours?.trim() ?? '',
    affiliate: {
      // Hidden until the settings load, so a switched-off program is never advertised.
      open: referralSettings?.enabled === true,
      rate: referralSettings?.enabled ? referralSettings.default_rate : null,
    },
    paymentMethodIds: paymentMethodIdsFromStatus(status),
  }
}

function paymentMethodIdsFromStatus(status: unknown): string[] | undefined {
  if (!status || typeof status !== 'object') return undefined
  const raw = (status as { payment_methods?: unknown }).payment_methods
  if (!Array.isArray(raw)) return undefined
  return raw.filter((item): item is string => typeof item === 'string')
}
