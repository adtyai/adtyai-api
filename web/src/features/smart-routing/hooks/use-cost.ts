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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  getCostGuards,
  getCostRankings,
  getCostSummary,
  getPrices,
  putPrice,
} from '../api'

export function usePrices() {
  return useQuery({
    queryKey: ['smart-routing', 'prices'],
    queryFn: getPrices,
    staleTime: 30_000,
  })
}

export function useCostRankings(enabled: boolean) {
  return useQuery({
    queryKey: ['smart-routing', 'cost-rankings'],
    queryFn: getCostRankings,
    enabled,
    staleTime: 60_000,
  })
}

export function useCostGuards() {
  return useQuery({
    queryKey: ['smart-routing', 'cost-guards'],
    queryFn: getCostGuards,
    staleTime: 30_000,
  })
}

export function useCostSummary(enabled: boolean) {
  return useQuery({
    queryKey: ['smart-routing', 'cost-summary'],
    queryFn: getCostSummary,
    enabled,
    staleTime: 60_000,
  })
}

export function usePutPrice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: putPrice,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['smart-routing', 'prices'] })
    },
  })
}
