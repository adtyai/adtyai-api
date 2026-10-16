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
}): Promise<{ rows: AffCommission[]; total: number }> {
  const res = await api.get<AffListResponse<AffCommission>>(
    '/api/aff/commissions/self',
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

export async function createAffWithdraw(input: {
  amount_quota: number
  idem_key: string
  pay_account: string
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
    `/api/aff/withdrawals/${  id}`,
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
