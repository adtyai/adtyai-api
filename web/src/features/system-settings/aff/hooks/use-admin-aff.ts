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

import type { AffAdminWithdrawAction } from '@/features/affiliate/lib/status'

import {
  adminAffWithdrawAction,
  adminApproveRiskReview,
  adminListAffCommissions,
  adminListAffSelfInviteReviews,
  adminListAffWithdrawals,
  adminRejectRiskReview,
  adminVoidAffCommission,
} from '../api'

export const ADMIN_AFF_COMMISSIONS_KEY = ['admin-aff-commissions'] as const
export const ADMIN_AFF_WITHDRAWALS_KEY = ['admin-aff-withdrawals'] as const
export const ADMIN_AFF_SELF_INVITE_KEY = ['admin-aff-self-invite'] as const

export function useAdminAffCommissions(status: string) {
  return useQuery({
    queryKey: [...ADMIN_AFF_COMMISSIONS_KEY, status || 'all'],
    queryFn: () =>
      adminListAffCommissions({
        status: status || undefined,
        limit: 50,
        offset: 0,
      }),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useVoidAffCommission() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => adminVoidAffCommission(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ADMIN_AFF_COMMISSIONS_KEY })
      }
    },
  })
}

export function useAdminAffWithdrawals(status: string) {
  return useQuery({
    queryKey: [...ADMIN_AFF_WITHDRAWALS_KEY, status || 'all'],
    queryFn: () =>
      adminListAffWithdrawals({
        status: status || undefined,
        limit: 50,
        offset: 0,
      }),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useAdminAffWithdrawAction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: {
      id: number
      action: AffAdminWithdrawAction
      reason?: string
    }) => adminAffWithdrawAction(args.id, args.action, { reason: args.reason }),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ADMIN_AFF_WITHDRAWALS_KEY })
      }
    },
  })
}

export function useAdminAffSelfInviteReviews(status: string) {
  return useQuery({
    queryKey: [...ADMIN_AFF_SELF_INVITE_KEY, status || 'all'],
    queryFn: () =>
      adminListAffSelfInviteReviews({
        status: status || undefined,
        limit: 50,
        offset: 0,
      }),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useApproveSelfInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => adminApproveRiskReview(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ADMIN_AFF_SELF_INVITE_KEY })
      }
    },
  })
}

export function useRejectSelfInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => adminRejectRiskReview(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: ADMIN_AFF_SELF_INVITE_KEY })
      }
    },
  })
}
