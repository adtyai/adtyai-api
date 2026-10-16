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

import { buildAdminOrdersQuery } from './lib/orders'
import type {
  AdminOrderCompleteResponse,
  AdminOrderDetailResponse,
  AdminOrdersListParams,
  AdminOrdersListResponse,
  TopupOrder,
} from './types'

function throwHttp(
  status: number,
  message?: string,
  code?: string
): never {
  const err = new Error(message || 'request failed') as Error & {
    status?: number
    code?: string
  }
  err.status = status
  if (code) err.code = code
  throw err
}

export async function listAdminOrders(
  params: AdminOrdersListParams
): Promise<{ rows: TopupOrder[]; total: number }> {
  const query = buildAdminOrdersQuery(params)
  const res = await api.get<AdminOrdersListResponse>('/api/admin/orders', {
    params: query,
    validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
  })
  if (res.status === 403) {
    throwHttp(403, res.data?.message || 'forbidden')
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}

export async function getAdminOrder(orderNo: string): Promise<TopupOrder> {
  const no = orderNo.trim()
  const res = await api.get<AdminOrderDetailResponse>(
    `/api/admin/orders/${  encodeURIComponent(no)}`,
    {
      validateStatus: (s) =>
        (s >= 200 && s < 300) || s === 403 || s === 404,
    }
  )
  if (res.status === 403) {
    throwHttp(403, res.data?.message || 'forbidden')
  }
  if (res.status === 404) {
    throwHttp(404, res.data?.message || 'order not found', 'not_found')
  }
  const body = requireServerSuccess(res.data)
  if (!body.data) throw new Error('empty order')
  return body.data
}

/**
 * Audit P1-1 contract: root-only; body must carry the gateway trade number
 * (`trade_no`) as evidence. 400 trade_no_required / 403 self_complete_forbidden /
 * 409 no_payment_attempt | trade_no_duplicate | order_status_invalid.
 */
export async function completeAdminOrder(
  orderNo: string,
  tradeNo: string
): Promise<AdminOrderCompleteResponse> {
  const res = await api.post<AdminOrderCompleteResponse>(
    `/api/admin/orders/${encodeURIComponent(orderNo.trim())}/complete`,
    { trade_no: tradeNo.trim() },
    {
      validateStatus: (s) =>
        (s >= 200 && s < 300) ||
        s === 400 ||
        s === 403 ||
        s === 404 ||
        s === 409,
    }
  )
  if (
    res.status === 400 ||
    res.status === 403 ||
    res.status === 404 ||
    res.status === 409
  ) {
    throwHttp(res.status, res.data?.message, res.data?.code)
  }
  return res.data
}
