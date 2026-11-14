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
import {
  createServerError,
  requireServerSuccess,
} from '@/lib/server-error-message'

type ApiResponse<T> = { success: boolean; message?: string; data?: T }

export type TeamRegistrationActor = 'root' | 'owner'

export type TeamRegistrationIssue = {
  id: number
  code: string
  site_id: number
  expires_at: number
  created_by: number
  created_at: number
  status: string
}

export type TeamRegistrationView = {
  id: number
  site_id: number
  created_by: number
  created_at: number
  expires_at: number
  used_by: number
  used_at: number
  team_id: number
  revoked_by: number
  revoked_at: number
  status: string
}

export type TeamRegistrationCreated = {
  Team?: { id?: number; name?: string }
  team?: { id?: number; name?: string }
}

const DEFAULT_TTL_HOURS = 24
export const TEAM_REGISTRATION_MAX_TTL_HOURS = 7 * 24
export const TEAM_REGISTRATION_CODE_MIN_LENGTH = 16

export function teamRegistrationCollectionPath(
  actor: TeamRegistrationActor
): string {
  return actor === 'root'
    ? '/api/adty/admin/team-registrations'
    : '/api/adty/site/team-registrations'
}

export function teamRegistrationUrl(
  code: string,
  origin = window.location.origin
): string {
  return `${origin}/team-register/${encodeURIComponent(code)}`
}

export function defaultTeamRegistrationTtlSeconds(): number {
  return DEFAULT_TTL_HOURS * 3600
}

export async function listTeamRegistrations(
  actor: TeamRegistrationActor
): Promise<TeamRegistrationView[]> {
  const res = await api.get<ApiResponse<{ items?: TeamRegistrationView[] }>>(
    teamRegistrationCollectionPath(actor)
  )
  return requireServerSuccess(res.data).data?.items ?? []
}

export async function issueTeamRegistration(
  actor: TeamRegistrationActor,
  ttlSeconds: number = defaultTeamRegistrationTtlSeconds()
): Promise<TeamRegistrationIssue> {
  const res = await api.post<ApiResponse<TeamRegistrationIssue>>(
    teamRegistrationCollectionPath(actor),
    { ttl_seconds: ttlSeconds }
  )
  const issued = requireServerSuccess(res.data).data
  if (!issued?.code) {
    throw createServerError(res.data, 'Failed to save')
  }
  return issued
}

export async function revokeTeamRegistration(
  actor: TeamRegistrationActor,
  id: number
): Promise<TeamRegistrationView> {
  const res = await api.post<ApiResponse<TeamRegistrationView>>(
    `${teamRegistrationCollectionPath(actor)}/${id}/revoke`
  )
  return requireServerSuccess(res.data).data as TeamRegistrationView
}

export async function completeTeamRegistration(body: {
  code: string
  username: string
  password: string
  name: string
}): Promise<TeamRegistrationCreated> {
  const res = await api.post<ApiResponse<TeamRegistrationCreated>>(
    '/api/adty/team-registrations/complete',
    body
  )
  return (requireServerSuccess(res.data).data ?? {}) as TeamRegistrationCreated
}
