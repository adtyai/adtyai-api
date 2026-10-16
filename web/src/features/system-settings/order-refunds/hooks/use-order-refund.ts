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

import { adminRefundTopupOrder, getTopupOrderByNo } from '../api'

export const ADMIN_ORDER_REFUND_KEY = ['admin-order-refund'] as const

export function useTopupOrderLookup(orderNo: string | null) {
  const no = (orderNo || '').trim()
  return useQuery({
    queryKey: [...ADMIN_ORDER_REFUND_KEY, 'order', no],
    queryFn: () => getTopupOrderByNo(no),
    enabled: no.length > 0,
    staleTime: 10_000,
    retry: (count, err) => {
      const status = (err as { status?: number })?.status
      if (status === 403 || status === 404) return false
      return count < 1
    },
  })
}

export function useAdminRefundTopupOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {
      orderNo: string
      amount: number
      reason: string
      request_id: string
    }) =>
      adminRefundTopupOrder(args.orderNo, {
        amount: args.amount,
        reason: args.reason,
        request_id: args.request_id,
        confirm: true,
      }),
    onSuccess: (data, vars) => {
      if (data.success && data.data?.order) {
        qc.setQueryData(
          [...ADMIN_ORDER_REFUND_KEY, 'order', vars.orderNo.trim()],
          data.data.order
        )
      }
      void qc.invalidateQueries({ queryKey: ADMIN_ORDER_REFUND_KEY })
    },
  })
}
