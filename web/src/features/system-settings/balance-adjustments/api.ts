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
  BalanceAdjustApproveResponse,
  BalanceAdjustListResponse,
  BalanceAdjustOneResponse,
  BalanceAdjustRequest,
} from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) ||
  s === 400 ||
  s === 403 ||
  s === 404 ||
  s === 409

export async function listBalanceAdjustments(params: {
  status?: string
  user_id?: number
  applicant_id?: number
  p?: number
  page_size?: number
}): Promise<{ rows: BalanceAdjustRequest[]; total: number }> {
  const res = await api.get<BalanceAdjustListResponse>(
    '/api/admin/balance-adjustments',
    {
      params: {
        status: params.status || undefined,
        user_id: params.user_id || undefined,
        applicant_id: params.applicant_id || undefined,
        p: params.p ?? 0,
        page_size: params.page_size ?? 50,
      },
      validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
    }
  )
  if (res.status === 403) {
    const err = new Error(
      (res.data as { message?: string })?.message || 'forbidden'
    ) as Error & { status?: number; code?: string }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function createBalanceAdjustment(body: {
  user_id: number
  account: 'quota' | 'bonus'
  amount: number
  reason: string
  chatwoot_conversation_id?: string
}): Promise<BalanceAdjustOneResponse> {
  const res = await api.post<BalanceAdjustOneResponse>(
    '/api/admin/balance-adjustments',
    body,
    { validateStatus: okish }
  )
  return res.data
}

export async function approveBalanceAdjustment(
  id: number,
  body: { confirm: true; self_approve_reason?: string }
): Promise<BalanceAdjustApproveResponse> {
  const res = await api.post<BalanceAdjustApproveResponse>(
    `/api/admin/balance-adjustments/${id}/approve`,
    body,
    { validateStatus: okish }
  )
  return res.data
}

export async function rejectBalanceAdjustment(
  id: number,
  body: { confirm: true; reason: string }
): Promise<BalanceAdjustOneResponse> {
  const res = await api.post<BalanceAdjustOneResponse>(
    `/api/admin/balance-adjustments/${id}/reject`,
    body,
    { validateStatus: okish }
  )
  return res.data
}

