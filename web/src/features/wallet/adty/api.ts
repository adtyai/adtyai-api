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

import type { AdtyOrder, AdtyPage, AdtyQuote, AdtyTopupInfo } from './types'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

export const adtyTopupQueryKeys = {
  info: ['adty-topup', 'info'] as const,
  order: (tradeNo: string) => ['adty-topup', 'order', tradeNo] as const,
  history: (page: number) => ['adty-topup', 'history', page] as const,
}

export async function getAdtyTopupInfo(): Promise<AdtyTopupInfo> {
  const res = await api.get<ApiResponse<AdtyTopupInfo>>('/api/adty/topup/info')
  return requireServerSuccess(res.data).data
}

export async function quoteAdtyTopup(
  amount: number,
  method: string,
  promoCode = '',
  paymentScene = 'wallet_topup'
): Promise<{ quote: AdtyQuote; available: boolean }> {
  const res = await api.post<
    ApiResponse<{ quote: AdtyQuote; available: boolean }>
  >('/api/adty/topup/quote', {
    amount,
    method,
    promo_code: promoCode,
    payment_scene: paymentScene,
  })
  return requireServerSuccess(res.data).data
}

export async function getAdtyOrder(tradeNo: string): Promise<AdtyOrder> {
  const res = await api.get<ApiResponse<AdtyOrder>>(
    `/api/adty/topup/orders/${encodeURIComponent(tradeNo)}`
  )
  return requireServerSuccess(res.data).data
}

export async function cancelAdtyOrder(tradeNo: string): Promise<void> {
  const res = await api.post<ApiResponse<null>>(
    `/api/adty/topup/orders/${encodeURIComponent(tradeNo)}/cancel`
  )
  requireServerSuccess(res.data)
}

export async function getAdtyTopupHistory(
  page: number,
  pageSize: number
): Promise<AdtyPage<AdtyOrder>> {
  const res = await api.get<ApiResponse<AdtyPage<AdtyOrder>>>(
    '/api/adty/topup/self',
    { params: { p: page, page_size: pageSize } }
  )
  return requireServerSuccess(res.data).data
}
