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
  approveRiskReview,
  getRiskReview,
  listRiskReviews,
  rejectRiskReview,
} from '../api'

export const RISK_REVIEWS_QUERY_KEY = ['admin-risk-reviews'] as const

export function useRiskReviews(status: string) {
  return useQuery({
    queryKey: [...RISK_REVIEWS_QUERY_KEY, status || 'all'],
    queryFn: () =>
      listRiskReviews({
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

export function useRiskReviewDetail(id: number | null) {
  return useQuery({
    queryKey: [...RISK_REVIEWS_QUERY_KEY, 'detail', id],
    queryFn: () => {
      if (id == null) {
        return Promise.reject(new Error('id required'))
      }
      return getRiskReview(id)
    },
    enabled: id != null && id > 0,
    staleTime: 10_000,
  })
}

export function useApproveRiskReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => approveRiskReview(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: RISK_REVIEWS_QUERY_KEY })
      }
    },
  })
}

export function useRejectRiskReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => rejectRiskReview(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: RISK_REVIEWS_QUERY_KEY })
      }
    },
  })
}
