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
  completeAdminOrder,
  getAdminOrder,
  listAdminOrders,
} from '../api'
import type { AdminOrdersListParams } from '../types'

export const ADMIN_ORDERS_QUERY_KEY = ['admin-topup-orders'] as const

export function useAdminOrders(params: AdminOrdersListParams) {
  return useQuery({
    queryKey: [...ADMIN_ORDERS_QUERY_KEY, 'list', params],
    queryFn: () => listAdminOrders(params),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useAdminOrderDetail(orderNo: string | null) {
  return useQuery({
    queryKey: [...ADMIN_ORDERS_QUERY_KEY, 'detail', orderNo],
    queryFn: () => {
      if (!orderNo) {
        return Promise.reject(new Error('orderNo required'))
      }
      return getAdminOrder(orderNo)
    },
    enabled: Boolean(orderNo && orderNo.trim()),
    staleTime: 10_000,
    retry: (count, err) => {
      const status = (err as { status?: number })?.status
      if (status === 403 || status === 404) return false
      return count < 2
    },
  })
}

export function useCompleteAdminOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: { orderNo: string; tradeNo: string }) =>
      completeAdminOrder(args.orderNo, args.tradeNo),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ADMIN_ORDERS_QUERY_KEY })
      }
    },
  })
}
