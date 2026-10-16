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

import type { AdminCreatePayload } from './lib/admins'

export type AdminCreateResult =
  | { ok: true; data?: { id: number; username: string } }
  | { ok: false; status: number; code?: string }

export async function createAdmin(
  payload: AdminCreatePayload
): Promise<AdminCreateResult> {
  const res = await api.post<{
    success: boolean
    code?: string
    data?: { id: number; username: string }
  }>('/api/admin/admins', payload, {
    validateStatus: () => true,
    // errors are mapped to fixed Chinese/English copy in the page
    skipErrorHandler: true,
  })
  if (res.status >= 200 && res.status < 300 && res.data?.success) {
    return { ok: true, data: res.data.data }
  }
  return { ok: false, status: res.status, code: res.data?.code }
}
