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

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { OfficialPrices } from '../lib/adty-savings'

type OfficialPricesResponse = {
  success: boolean
  message?: string
  data: { prices: OfficialPrices; updated_at: number }
}

export const officialPricesQueryKey = ['adty-official-prices'] as const

const NO_PRICES: OfficialPrices = {}

/**
 * Official list prices for the pricing page (adty task 8.4). The page still
 * works without them, so a failed request only hides the savings.
 */
export function useOfficialPrices() {
  const query = useQuery({
    queryKey: officialPricesQueryKey,
    queryFn: async () => {
      const res = await api.get<OfficialPricesResponse>(
        '/api/adty/pricing/official'
      )
      return requireServerSuccess(res.data).data
    },
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  return {
    prices: query.data?.prices ?? NO_PRICES,
    updatedAt: query.data?.updated_at ?? 0,
    isLoading: query.isPending,
  }
}
