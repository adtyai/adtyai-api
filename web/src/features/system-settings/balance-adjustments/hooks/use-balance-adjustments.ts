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
  approveBalanceAdjustment,
  createBalanceAdjustment,
  listBalanceAdjustments,
  rejectBalanceAdjustment,
} from '../api'

export const BALANCE_ADJUSTMENTS_KEY = ['admin-balance-adjustments'] as const

export function useBalanceAdjustments(status: string) {
  return useQuery({
    queryKey: [...BALANCE_ADJUSTMENTS_KEY, status || 'all'],
    queryFn: () =>
      listBalanceAdjustments({
        status: status || undefined,
        page_size: 50,
        p: 0,
      }),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useCreateBalanceAdjustment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: {
      user_id: number
      account: 'quota' | 'bonus'
      amount: number
      reason: string
      chatwoot_conversation_id?: string
    }) => createBalanceAdjustment(body),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: BALANCE_ADJUSTMENTS_KEY })
      }
    },
  })
}

export function useApproveBalanceAdjustment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {
      id: number
      self_approve_reason?: string
    }) =>
      approveBalanceAdjustment(args.id, {
        confirm: true,
        self_approve_reason: args.self_approve_reason,
      }),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: BALANCE_ADJUSTMENTS_KEY })
      }
    },
  })
}

export function useRejectBalanceAdjustment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: { id: number; reason: string }) =>
      rejectBalanceAdjustment(args.id, {
        confirm: true,
        reason: args.reason,
      }),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: BALANCE_ADJUSTMENTS_KEY })
      }
    },
  })
}
