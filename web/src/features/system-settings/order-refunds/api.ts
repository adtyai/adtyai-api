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

import type { OrderOneResponse, RefundOneResponse, TopupOrder } from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

export async function getTopupOrderByNo(orderNo: string): Promise<TopupOrder> {
  const no = orderNo.trim()
  const res = await api.get<OrderOneResponse>(
    `/api/topup/order/${encodeURIComponent(no)}`,
    {
      validateStatus: (s) => (s >= 200 && s < 300) || s === 403 || s === 404,
    }
  )
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & {
      status?: number
      code?: string
    }
    err.status = 403
    throw err
  }
  if (res.status === 404 || !res.data?.success || !res.data.data) {
    const err = new Error(res.data?.message || 'order not found', {
      cause: res.data,
    }) as Error & {
      status?: number
      code?: string
    }
    err.status = 404
    err.code = 'not_found'
    throw err
  }
  return res.data.data
}

export async function adminRefundTopupOrder(
  orderNo: string,
  body: {
    amount: number
    reason: string
    request_id: string
    confirm: true
  }
): Promise<RefundOneResponse> {
  const res = await api.post<RefundOneResponse>(
    `/api/admin/orders/${encodeURIComponent(orderNo.trim())}/refund`,
    body,
    { validateStatus: okish }
  )
  return res.data
}
