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

import { getPricing } from '../api'
import type { PricingData, PricingModel } from '../types'

let pricedModels: PricingModel[] = []
let pricedFrom: PricingData | undefined

export function modelsFromPricing(data: PricingData | undefined): PricingModel[] {
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

export function usePricingData(enabled = true) {
  const { status } = useStatus()

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['pricing'],
    queryFn: async () => requireServerSuccess(await getPricing()),
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
