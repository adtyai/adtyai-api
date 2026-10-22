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
  AffCommission,
  AffListResponse,
  AffOneResponse,
  AffSelfSummary,
  AffWithdrawal,
} from './types'

export async function getAffSelf(): Promise<AffSelfSummary> {
  const res = await api.get<AffOneResponse<AffSelfSummary>>('/api/aff/self')
  const body = requireServerSuccess(res.data)
  if (!body.data) throw new Error('empty')
  return body.data
}

export async function listAffCommissionsSelf(params?: {
  limit?: number
  offset?: number
  status?: string
}): Promise<{ rows: AffCommission[]; total: number }> {
  const res = await api.get<AffListResponse<AffCommission>>(
    '/api/aff/commissions/self',
    {
      params: {
        limit: params?.limit ?? 50,
        offset: params?.offset ?? 0,
        status: params?.status || undefined,
      },
    }
  )
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function createAffWithdraw(input: {
  amount_quota: number
  idem_key: string
  pay_method: 'usdt' | 'alipay'
  pay_account: string
  usdt_chain: string
  payout_qr_id?: number
}): Promise<AffOneResponse<AffWithdrawal>> {
  const res = await api.post<AffOneResponse<AffWithdrawal>>(
    '/api/aff/withdraw',
    input,
    {
      validateStatus: (s) =>
        (s >= 200 && s < 300) || s === 400 || s === 403 || s === 409,
    }
  )
  return res.data
}

export async function fetchAffPayoutQrObjectUrl(id: number): Promise<string> {
  const res = await api.get(`/api/aff/payout-qr/${id}`, {
    responseType: 'blob',
    validateStatus: (s) => s === 200 || s === 404,
  })
  const contentType = String(res.headers['content-type'] || '')
  if (res.status !== 200 || !contentType.startsWith('image/')) {
    throw new Error('invalid payout qr')
  }
  return URL.createObjectURL(res.data)
}

export async function uploadAffPayoutQr(
  file: File
): Promise<AffOneResponse<{ id: number; sha256: string }>> {
  const body = new FormData()
  body.append('file', file)
  const res = await api.post<AffOneResponse<{ id: number; sha256: string }>>(
    '/api/aff/payout-qr',
    body,
    {
      validateStatus: (s) =>
        (s >= 200 && s < 300) || s === 400 || s === 403 || s === 413,
    }
  )
  return res.data
}

export type AffInvitee = {
  user_id: number
  name: string
  registered_at: number
  paid_usd: number
  commission_quota: number
  status: string
}

export async function listAffInvitees(params?: {
  limit?: number
  offset?: number
}): Promise<{ rows: AffInvitee[]; total: number }> {
  const res = await api.get<AffListResponse<AffInvitee>>('/api/aff/invitees', {
    params: {
      limit: params?.limit ?? 20,
      offset: params?.offset ?? 0,
    },
  })
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function listAffWithdrawalsSelf(params?: {
  limit?: number
  offset?: number
}): Promise<{ rows: AffWithdrawal[]; total: number }> {
  const res = await api.get<AffListResponse<AffWithdrawal>>(
    '/api/aff/withdrawals',
    {
      params: {
        limit: params?.limit ?? 50,
        offset: params?.offset ?? 0,
      },
    }
  )
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function getAffWithdrawalSelf(id: number): Promise<AffWithdrawal> {
  const res = await api.get<AffOneResponse<AffWithdrawal>>(
    `/api/aff/withdrawals/${id}`,
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
