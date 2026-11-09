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

type ApiResponse<T> = { success: boolean; message?: string; data: T }
type Page<T> = { items: T[] | null; total: number }
const ADMIN = '/api/adty/admin/promo-codes'
export const promoKeys = {
  all: ['adty-promo'] as const,
  promoCodes: (page: number) => ['adty-promo', 'codes', page] as const,
}

/** Promo code (adty task 12.4). Money and percent values are decimal strings. */
export type PromoCode = {
  id: number
  code: string
  kind: 'percent' | 'fixed'
  value: string
  min_face_usd: string
  max_uses: number
  per_user_limit: number
  used_count: number
  starts_at: number
  expires_at: number
  new_users_only: boolean
  site_id: number
  funder: 'platform' | 'site_owner'
  enabled: boolean
  note: string
}

export type PromoCodeInput = Omit<PromoCode, 'id' | 'used_count' | 'funder'>

export async function getPromoCodes(page: number): Promise<Page<PromoCode>> {
  const res = await api.get<ApiResponse<Page<PromoCode>>>(`${ADMIN}`, {
    params: { p: page, page_size: 20 },
  })
  return requireServerSuccess(res.data).data
}

export async function savePromoCode(
  id: number | null,
  input: PromoCodeInput
): Promise<PromoCode> {
  const res = id
    ? await api.put<ApiResponse<PromoCode>>(`${ADMIN}/${id}`, input)
    : await api.post<ApiResponse<PromoCode>>(`${ADMIN}`, input)
  return requireServerSuccess(res.data).data
}
