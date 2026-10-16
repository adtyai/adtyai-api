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

import { getBusinessParams, putBusinessParam } from '../api'

export const BUSINESS_PARAMS_QUERY_KEY = ['admin-business-params'] as const

export function useBusinessParams() {
  return useQuery({
    queryKey: BUSINESS_PARAMS_QUERY_KEY,
    queryFn: getBusinessParams,
    staleTime: 30_000,
  })
}

export function useUpdateBusinessParam() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      key,
      value,
    }: {
      key: string
      value: string | number | boolean | unknown
    }) => putBusinessParam(key, value),
    onSuccess: (data) => {
      if (data.success) {
        void queryClient.invalidateQueries({
          queryKey: BUSINESS_PARAMS_QUERY_KEY,
        })
      }
    },
  })
}
