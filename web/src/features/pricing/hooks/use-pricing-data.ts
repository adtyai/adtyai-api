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

import { useStatus } from '@/hooks/use-status'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getPricing, pricingDisplayGroupParam } from '../api'
import { FILTER_ALL } from '../constants'
import type { PricingData, PricingModel } from '../types'

/**
 * Group used to fetch a price list adjusted for the current display group
 * (ISS-R3-0009). "all" has no single adjustment group, so the request omits
 * the parameter and the backend keeps the viewer's current group.
 */
export function resolvePricingDisplayGroup(
  explicitGroup?: string,
  userGroup?: string
): string | undefined {
  if (explicitGroup === FILTER_ALL) return undefined
  return pricingDisplayGroupParam(explicitGroup || userGroup || 'default')
}

let pricedModels: PricingModel[] = []
let pricedFrom: PricingData | undefined

export function modelsFromPricing(
  data: PricingData | undefined
): PricingModel[] {
  if (!Array.isArray(data?.data)) return []
  if (
    pricedFrom?.data === data.data &&
    pricedFrom.vendors === data.vendors &&
    pricedFrom.group_ratio === data.group_ratio
  ) {
    return pricedModels
  }
  const groupRatio = data.group_ratio
  const vendors = new Map(data.vendors?.map((vendor) => [vendor.id, vendor]))
  pricedModels = data.data.map((model) => {
    const vendor = model.vendor_id ? vendors.get(model.vendor_id) : undefined
    return {
      ...model,
      key: model.model_name,
      vendor_name: vendor?.name,
      vendor_icon: vendor?.icon,
      vendor_description: vendor?.description,
      group_ratio: groupRatio,
    }
  })
  pricedFrom = data
  return pricedModels
}

export function usePricingData(enabled = true, displayGroup?: string) {
  const { status } = useStatus()
  const groupParam = pricingDisplayGroupParam(displayGroup)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: groupParam ? ['pricing', groupParam] : ['pricing'],
    queryFn: async () => requireServerSuccess(await getPricing(groupParam)),
    staleTime: 5 * 60 * 1000,
    enabled,
  })

  // Ensure rates never reach zero to prevent division errors
  const priceRate = useMemo(
    () => Math.max((status?.price as number) ?? 1, 0.001),
    [status?.price]
  )
  const usdExchangeRate = useMemo(
    () => Math.max((status?.usd_exchange_rate as number) ?? priceRate, 0.001),
    [status?.usd_exchange_rate, priceRate]
  )

  const models = useMemo(() => modelsFromPricing(data), [data])

  return {
    models,
    vendors: data?.vendors ?? [],
    groupRatio: data?.group_ratio ?? {},
    usableGroup: data?.usable_group ?? {},
    endpointMap: data?.supported_endpoint ?? {},
    autoGroups: data?.auto_groups ?? [],
    isLoading,
    error,
    refetch,
    priceRate,
    usdExchangeRate,
  }
}
