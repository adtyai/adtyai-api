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
  createEnterprisePricing,
  deleteEnterprisePricing,
  listEnterprisePricing,
  updateEnterprisePricing,
} from '../api'
import type {
  EnterprisePriceCreateInput,
  EnterprisePriceUpdateInput,
} from '../types'

export const ENTERPRISE_PRICING_KEY = ['admin-enterprise-pricing'] as const

export function useEnterprisePricing(userId: number | undefined, page: number) {
  return useQuery({
    queryKey: [...ENTERPRISE_PRICING_KEY, userId || 0, page],
    queryFn: () =>
      listEnterprisePricing({
        user_id: userId,
        p: page * 50,
        page_size: 50,
      }),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useCreateEnterprisePricing() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: EnterprisePriceCreateInput) =>
      createEnterprisePricing(input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ENTERPRISE_PRICING_KEY })
      }
    },
  })
}

export function useUpdateEnterprisePricing() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number
      input: EnterprisePriceUpdateInput
    }) => updateEnterprisePricing(id, input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ENTERPRISE_PRICING_KEY })
      }
    },
  })
}

export function useDeleteEnterprisePricing() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deleteEnterprisePricing(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ENTERPRISE_PRICING_KEY })
      }
    },
  })
}
