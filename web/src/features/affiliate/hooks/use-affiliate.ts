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
  createAffWithdraw,
  getAffSelf,
  listAffCommissionsSelf,
  listAffWithdrawalsSelf,
} from '../api'

export const AFF_SELF_QUERY_KEY = ['aff-self'] as const
export const AFF_COMMISSIONS_QUERY_KEY = ['aff-commissions-self'] as const
export const AFF_WITHDRAWALS_QUERY_KEY = ['aff-withdrawals-self'] as const

export function useAffSelf() {
  return useQuery({
    queryKey: AFF_SELF_QUERY_KEY,
    queryFn: getAffSelf,
    staleTime: 15_000,
  })
}

export function useAffCommissionsSelf(params?: {
  limit?: number
  offset?: number
}) {
  return useQuery({
    queryKey: [...AFF_COMMISSIONS_QUERY_KEY, params?.limit, params?.offset],
    queryFn: () => listAffCommissionsSelf(params),
    staleTime: 15_000,
  })
}

export function useAffWithdrawalsSelf(params?: {
  limit?: number
  offset?: number
}) {
  return useQuery({
    queryKey: [...AFF_WITHDRAWALS_QUERY_KEY, params?.limit, params?.offset],
    queryFn: () => listAffWithdrawalsSelf(params),
    staleTime: 15_000,
  })
}

export function useCreateAffWithdraw() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createAffWithdraw,
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: AFF_SELF_QUERY_KEY })
        void qc.invalidateQueries({ queryKey: AFF_WITHDRAWALS_QUERY_KEY })
        void qc.invalidateQueries({ queryKey: AFF_COMMISSIONS_QUERY_KEY })
      }
    },
  })
}
