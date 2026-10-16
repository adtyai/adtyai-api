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

import {
  getBreakerEvents,
  getBreakerStages,
  getRankings,
  getScores,
} from '../api'

export function useScores(model?: string) {
  return useQuery({
    queryKey: ['smart-routing', 'scores', model ?? ''],
    queryFn: () => getScores(model ? { model } : undefined),
    staleTime: 30_000,
  })
}

export function useRankings(model?: string) {
  return useQuery({
    queryKey: ['smart-routing', 'rankings', model ?? ''],
    queryFn: () => getRankings(model ? { model } : undefined),
    staleTime: 30_000,
  })
}

export function useBreakerStages() {
  return useQuery({
    queryKey: ['smart-routing', 'breaker-stages'],
    queryFn: getBreakerStages,
    staleTime: 15_000,
    refetchInterval: 15_000,
  })
}

export function useBreakerEvents() {
  return useQuery({
    queryKey: ['smart-routing', 'breaker-events'],
    queryFn: getBreakerEvents,
    staleTime: 30_000,
  })
}
