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

import { getMenuVisibility, putMenuVisibility } from '../api'
import type { MenuVisibilityPutInput } from '../types'

export const MENU_VISIBILITY_QUERY_KEY = ['admin-menu-visibility'] as const

export function useMenuVisibility() {
  return useQuery({
    queryKey: MENU_VISIBILITY_QUERY_KEY,
    queryFn: getMenuVisibility,
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function usePutMenuVisibility() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: MenuVisibilityPutInput) => putMenuVisibility(input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: MENU_VISIBILITY_QUERY_KEY })
      }
    },
  })
}
