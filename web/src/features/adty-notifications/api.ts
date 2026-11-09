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

import type { NotificationInput } from './lib/schema'

export type NotificationMode = 'root' | 'owner'
export type Publication = NotificationInput & {
  id: number
  revision: number
  publisher_id: number
  publisher_name: string
  issuer_site_id: number
  target_site_name: string
  published_at: number
  status: 'published' | 'withdrawn'
}
export const notificationManagerKey = ['adty-notification-manager'] as const

function base(mode: NotificationMode) {
  return mode === 'root'
    ? '/api/adty/admin/notifications'
    : '/api/adty/site/notifications'
}

export async function getPublications(
  mode: NotificationMode,
  page: number
): Promise<{ items: Publication[]; total: number }> {
  const response = await api.get(base(mode), {
    params: { p: page, page_size: 20 },
  })
  return requireServerSuccess(response.data).data
}

export async function savePublication(
  mode: NotificationMode,
  row: Publication | null,
  input: NotificationInput
): Promise<void> {
  const body = { ...input, revision: row?.revision ?? 0 }
  const response = row
    ? await api.put(`${base(mode)}/${row.id}`, body)
    : await api.post(base(mode), body)
  requireServerSuccess(response.data)
}

export async function withdrawPublication(
  mode: NotificationMode,
  row: Publication
): Promise<void> {
  const response = await api.delete(`${base(mode)}/${row.id}`, {
    data: { revision: row.revision },
  })
  requireServerSuccess(response.data)
}
