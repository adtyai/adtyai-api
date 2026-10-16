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
  getRoutingConfig,
  getWeightHistory,
  getWeightSchemes,
  putRoutingConfig,
} from '../api'

export const ROUTING_CONFIG_QUERY_KEY = ['smart-routing', 'config'] as const
export const WEIGHT_SCHEMES_QUERY_KEY = ['smart-routing', 'weight-schemes'] as const
export const WEIGHT_HISTORY_QUERY_KEY = ['smart-routing', 'weight-history'] as const

export function useRoutingConfig() {
  return useQuery({
    queryKey: ROUTING_CONFIG_QUERY_KEY,
    queryFn: getRoutingConfig,
    staleTime: 30_000,
  })
}

export function useWeightSchemes() {
  return useQuery({
    queryKey: WEIGHT_SCHEMES_QUERY_KEY,
    queryFn: getWeightSchemes,
    staleTime: 30_000,
  })
}

export function useWeightHistory() {
  return useQuery({
    queryKey: WEIGHT_HISTORY_QUERY_KEY,
    queryFn: getWeightHistory,
    staleTime: 30_000,
  })
}

export function usePutRoutingConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: putRoutingConfig,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ROUTING_CONFIG_QUERY_KEY })
    },
  })
}

/** True when smart routing total switch is on (for channels page). */
export function useSmartRoutingEnabled(): boolean {
  const q = useRoutingConfig()
  return Boolean(q.data?.enabled)
}
