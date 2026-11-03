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
import type { ContentLang } from '@/lib/adty-lang'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type GroupTexts = Partial<Record<ContentLang, string>>

export type GroupDisplayItem = {
  id: string
  names: GroupTexts
  descs: GroupTexts
  usable: boolean
  visible: boolean
}

export type GroupRefs = {
  users: number
  tokens: number
  channels: number
}

export type AdminGroup = {
  id: string
  names: GroupTexts
  descs: GroupTexts
  refs: GroupRefs
}

type ApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export const groupDisplayQueryKey = ['adty-groups', 'display'] as const
export const adminGroupsQueryKey = ['adty-groups', 'admin'] as const

export async function getGroupDisplay(): Promise<GroupDisplayItem[]> {
  const res = await api.get<ApiResponse<GroupDisplayItem[]>>(
    '/api/adty/groups/display'
  )
  return requireServerSuccess(res.data).data ?? []
}

export async function getAdminGroups(): Promise<AdminGroup[]> {
  const res = await api.get<ApiResponse<AdminGroup[]>>('/api/adty/admin/groups')
  return requireServerSuccess(res.data).data ?? []
}

export async function createAdminGroup(body: {
  name: string
  desc: string
}): Promise<AdminGroup> {
  const res = await api.post<ApiResponse<AdminGroup>>(
    '/api/adty/admin/groups',
    body
  )
  return requireServerSuccess(res.data).data
}

export async function updateAdminGroup(
  id: string,
  body: { names: GroupTexts; descs: GroupTexts }
): Promise<AdminGroup> {
  const res = await api.put<ApiResponse<AdminGroup>>(
    `/api/adty/admin/groups/${encodeURIComponent(id)}`,
    body
  )
  return requireServerSuccess(res.data).data
}

export async function migrateAdminGroup(
  id: string,
  to: string
): Promise<GroupRefs> {
  const res = await api.post<ApiResponse<GroupRefs>>(
    `/api/adty/admin/groups/${encodeURIComponent(id)}/migrate`,
    { to }
  )
  return requireServerSuccess(res.data).data
}

export async function deleteAdminGroup(id: string): Promise<void> {
  const res = await api.delete<ApiResponse<null>>(
    `/api/adty/admin/groups/${encodeURIComponent(id)}`
  )
  requireServerSuccess(res.data)
}
