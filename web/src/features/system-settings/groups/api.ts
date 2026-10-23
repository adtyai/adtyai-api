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
  GroupGrantListResponse,
  GroupGrantWriteResponse,
  GroupMetaCreateInput,
  GroupMetaListResponse,
  GroupMetaOneResponse,
  GroupMetaPutInput,
  GroupMetaView,
} from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

export async function listGroupMetas(): Promise<GroupMetaView[]> {
  const res = await api.get<GroupMetaListResponse>('/api/admin/groups', {
    validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
  })
  if (res.status === 403) {
    const err = new Error(res.data?.message || 'forbidden', {
      cause: res.data,
    }) as Error & {
      status?: number
    }
    err.status = 403
    throw err
  }
  const body = requireServerSuccess(res.data)
  return body.data ?? []
}

export async function createGroupMeta(
  input: GroupMetaCreateInput
): Promise<GroupMetaOneResponse> {
  const res = await api.post<GroupMetaOneResponse>('/api/admin/groups', input, {
    validateStatus: okish,
  })
  return res.data
}

export async function putGroupMeta(
  code: string,
  input: GroupMetaPutInput
): Promise<GroupMetaOneResponse> {
  const res = await api.put<GroupMetaOneResponse>(
    `/api/admin/groups/${encodeURIComponent(code)}`,
    input,
    { validateStatus: okish }
  )
  return res.data
}

export async function listGroupGrants(
  code: string
): Promise<GroupGrantListResponse> {
  const res = await api.get<GroupGrantListResponse>(
    `/api/admin/groups/${encodeURIComponent(code)}/grants`,
    { validateStatus: okish }
  )
  return res.data
}

export async function grantHiddenGroup(
  code: string,
  userId: number
): Promise<GroupGrantWriteResponse> {
  const res = await api.post<GroupGrantWriteResponse>(
    `/api/admin/groups/${encodeURIComponent(code)}/grants`,
    { user_id: userId },
    { validateStatus: okish }
  )
  return res.data
}

export async function saveGroupBilling(input: {
  group_ratio: Record<string, unknown>
  user_usable_groups: Record<string, unknown>
  display_names: Record<string, string>
}): Promise<{ success: boolean; message?: string }> {
  const res = await api.put<{ success: boolean; message?: string }>(
    '/api/admin/groups/billing',
    input,
    { validateStatus: (s) => okish(s) || s === 500 }
  )
  return res.data
}

export async function revokeHiddenGroup(
  code: string,
  userId: number
): Promise<GroupGrantWriteResponse> {
  const res = await api.delete<GroupGrantWriteResponse>(
    `/api/admin/groups/${encodeURIComponent(code)}/grants/${userId}`,
    { validateStatus: okish }
  )
  return res.data
}
