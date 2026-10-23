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
  AffAdminUserDetail,
  AffCommission,
  AffInviterRow,
  AffListResponse,
  AffOneResponse,
  AffOverview,
  AffRiskReview,
  AffWithdrawal,
} from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

function forbidOrOk(status: number) {
  return (status >= 200 && status < 300) || status === 403
}

export async function adminListAffCommissions(params: {
  status?: string
  inviter_id?: number
  keyword?: string
  limit?: number
  offset?: number
}): Promise<{ rows: AffCommission[]; total: number }> {
  const res = await api.get<AffListResponse<AffCommission>>(
    '/api/admin/aff/commissions',
    {
      params: {
        status: params.status || undefined,
        inviter_id: params.inviter_id || undefined,
        keyword: params.keyword || undefined,
        limit: params.limit ?? 50,
        offset: params.offset ?? 0,
      },
      validateStatus: forbidOrOk,
    }
  )
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & {
      status?: number
    }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function adminVoidAffCommission(
  id: number
): Promise<AffOneResponse<AffCommission>> {
  const res = await api.post<AffOneResponse<AffCommission>>(
    `/api/admin/aff/commissions/${id}/void`,
    { confirm: true },
    { validateStatus: okish }
  )
  return res.data
}

export async function adminListAffWithdrawals(params: {
  status?: string
  pay_method?: string
  keyword?: string
  limit?: number
  offset?: number
}): Promise<{
  rows: AffWithdrawal[]
  total: number
  counts: Record<string, number>
}> {
  const res = await api.get<AffListResponse<AffWithdrawal>>(
    '/api/admin/aff/withdrawals',
    {
      params: {
        status: params.status || undefined,
        pay_method: params.pay_method || undefined,
        keyword: params.keyword || undefined,
        limit: params.limit ?? 50,
        offset: params.offset ?? 0,
      },
      validateStatus: forbidOrOk,
    }
  )
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & {
      status?: number
    }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return {
    rows: body.data ?? [],
    total: body.total ?? 0,
    counts: body.counts ?? {},
  }
}

export async function adminListAffInviters(params: {
  keyword?: string
  limit?: number
  offset?: number
}): Promise<{ rows: AffInviterRow[]; total: number }> {
  const res = await api.get<AffListResponse<AffInviterRow>>(
    '/api/admin/aff/inviters',
    {
      params: {
        keyword: params.keyword || undefined,
        limit: params.limit ?? 20,
        offset: params.offset ?? 0,
      },
      validateStatus: forbidOrOk,
    }
  )
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & { status?: number }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function adminGetAffOverview(): Promise<AffOverview> {
  const res = await api.get<AffOneResponse<AffOverview>>(
    '/api/admin/aff/overview',
    { validateStatus: forbidOrOk }
  )
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & { status?: number }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  if (!body.data) throw new Error('empty')
  return body.data
}

export async function adminGetAffUserDetail(
  id: number,
  offsets?: {
    invitee_offset?: number
    commission_offset?: number
    withdrawal_offset?: number
    transfer_offset?: number
  }
): Promise<AffAdminUserDetail> {
  const res = await api.get<AffOneResponse<AffAdminUserDetail>>(
    `/api/admin/aff/users/${id}`,
    {
      params: {
        limit: 20,
        invitee_offset: offsets?.invitee_offset ?? 0,
        commission_offset: offsets?.commission_offset ?? 0,
        withdrawal_offset: offsets?.withdrawal_offset ?? 0,
        transfer_offset: offsets?.transfer_offset ?? 0,
      },
      validateStatus: (s) => forbidOrOk(s) || s === 404,
    }
  )
  if (res.status === 403 || res.status === 404) {
    const err = new Error(res.data?.message || 'failed', {
      cause: res.data,
    }) as Error & { status?: number }
    err.status = res.status
    throw err
  }
  const body = requireServerSuccess(res.data)
  if (!body.data) throw new Error('empty')
  return body.data
}

export async function adminAffWithdrawAction(
  id: number,
  action:
    | 'approve'
    | 'reject'
    | 'mark-paid'
    | 'mark-failed'
    | 'mark-unknown'
    | 'retry',
  extra?: { reason?: string; payout_ref?: string }
): Promise<AffOneResponse<AffWithdrawal>> {
  const body: { confirm: true; reason?: string; payout_ref?: string } = {
    confirm: true,
  }
  if (action === 'reject' && extra?.reason) body.reason = extra.reason
  if (action === 'mark-paid' && extra?.payout_ref) {
    body.payout_ref = extra.payout_ref
  }
  const res = await api.post<AffOneResponse<AffWithdrawal>>(
    `/api/admin/aff/withdrawals/${id}/${action}`,
    body,
    { validateStatus: okish }
  )
  return res.data
}

export async function adminListAffSelfInviteReviews(params: {
  status?: string
  limit?: number
  offset?: number
}): Promise<{ rows: AffRiskReview[]; total: number }> {
  const res = await api.get<AffListResponse<AffRiskReview>>(
    '/api/admin/aff/self-invite-reviews',
    {
      params: {
        status: params.status || undefined,
        limit: params.limit ?? 50,
        offset: params.offset ?? 0,
      },
      validateStatus: forbidOrOk,
    }
  )
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & {
      status?: number
    }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function adminApproveRiskReview(
  id: number
): Promise<{ success: boolean; message?: string; code?: string }> {
  const res = await api.post(
    `/api/admin/risk-reviews/${id}/approve`,
    { confirm: true },
    { validateStatus: okish }
  )
  return res.data
}

export async function adminRejectRiskReview(
  id: number
): Promise<{ success: boolean; message?: string; code?: string }> {
  const res = await api.post(
    `/api/admin/risk-reviews/${id}/reject`,
    { confirm: true },
    { validateStatus: okish }
  )
  return res.data
}

export async function adminReopenRiskReview(
  id: number
): Promise<{ success: boolean; message?: string; code?: string }> {
  const res = await api.post(
    `/api/admin/risk-reviews/${id}/reopen`,
    {},
    { validateStatus: okish }
  )
  return res.data
}
