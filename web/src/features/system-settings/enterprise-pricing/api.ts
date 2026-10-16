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
  EnterprisePriceCreateInput,
  EnterprisePriceListResponse,
  EnterprisePriceOneResponse,
  EnterprisePriceRule,
  EnterprisePriceUpdateInput,
} from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

export async function listEnterprisePricing(params: {
  user_id?: number
  p?: number
  page_size?: number
}): Promise<{ rows: EnterprisePriceRule[]; total: number }> {
  const res = await api.get<EnterprisePriceListResponse>(
    '/api/admin/enterprise-pricing',
    {
      params: {
        user_id: params.user_id || undefined,
        p: params.p ?? 0,
        page_size: params.page_size ?? 50,
      },
      validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
    }
  )
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

export async function createEnterprisePricing(
  input: EnterprisePriceCreateInput
): Promise<EnterprisePriceOneResponse> {
  const res = await api.post<EnterprisePriceOneResponse>(
    '/api/admin/enterprise-pricing',
    input,
    { validateStatus: okish }
  )
  return res.data
}

export async function updateEnterprisePricing(
  id: number,
  input: EnterprisePriceUpdateInput
): Promise<EnterprisePriceOneResponse> {
  const res = await api.put<EnterprisePriceOneResponse>(
    `/api/admin/enterprise-pricing/${id}`,
    input,
    { validateStatus: okish }
  )
  return res.data
}

export async function deleteEnterprisePricing(
  id: number
): Promise<EnterprisePriceOneResponse> {
  const res = await api.delete<EnterprisePriceOneResponse>(
    `/api/admin/enterprise-pricing/${id}`,
    {
      data: { confirm: true },
      validateStatus: okish,
    }
  )
  return res.data
}
