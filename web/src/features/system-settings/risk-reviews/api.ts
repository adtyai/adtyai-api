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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  RiskReview,
  RiskReviewActionResponse,
  RiskReviewDetailResponse,
  RiskReviewListResponse,
} from './types'

export async function listRiskReviews(params: {
  status?: string
  limit?: number
  offset?: number
}): Promise<{ rows: RiskReview[]; total: number }> {
  const res = await api.get<RiskReviewListResponse>('/api/admin/risk-reviews', {
    params: {
      status: params.status || undefined,
      limit: params.limit ?? 50,
      offset: params.offset ?? 0,
    },
    validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
  })
  if (res.status === 403) {
    const err = new Error(
      (res.data as { message?: string })?.message || 'forbidden'
    ) as Error & { status?: number }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function getRiskReview(id: number): Promise<RiskReview> {
  const res = await api.get<RiskReviewDetailResponse>(
    `/api/admin/risk-reviews/${id}`,
    {
      validateStatus: (s) => (s >= 200 && s < 300) || s === 403 || s === 404,
    }
  )
  if (res.status === 403 || res.status === 404) {
    const err = new Error(res.data?.message || 'failed', {
      cause: res.data,
    }) as Error & {
      status?: number
    }
    err.status = res.status
    throw err
  }
  const body = requireServerSuccess(res.data)
  if (!body.data) throw new Error('empty')
  return body.data
}

export async function approveRiskReview(
  id: number
): Promise<RiskReviewActionResponse> {
  const res = await api.post<RiskReviewActionResponse>(
    `/api/admin/risk-reviews/${id}/approve`,
    { confirm: true },
    {
      validateStatus: (s) =>
        (s >= 200 && s < 300) ||
        s === 400 ||
        s === 403 ||
        s === 404 ||
        s === 409,
    }
  )
  return res.data
}

export async function rejectRiskReview(
  id: number
): Promise<RiskReviewActionResponse> {
  const res = await api.post<RiskReviewActionResponse>(
    `/api/admin/risk-reviews/${id}/reject`,
    { confirm: true },
    {
      validateStatus: (s) =>
        (s >= 200 && s < 300) ||
        s === 400 ||
        s === 403 ||
        s === 404 ||
        s === 409,
    }
  )
  return res.data
}
