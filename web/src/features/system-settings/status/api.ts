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

import type { HealthGetResponse, HealthSummary } from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

function throwIfForbidden(status: number, message?: string): void {
  if (status !== 403) return
  const err = new Error(message || 'forbidden') as Error & { status?: number }
  err.status = 403
  throw err
}

export async function getHealthSummary(): Promise<HealthSummary> {
  const res = await api.get<HealthGetResponse>('/api/admin/health', {
    validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
  })
  throwIfForbidden(res.status, res.data?.message)
  const body = requireServerSuccess(res.data)
  if (!body.data) {
    throw new Error('health summary missing')
  }
  return body.data
}

export async function refreshHealthSummary(): Promise<HealthSummary> {
  const res = await api.post<HealthGetResponse>(
    '/api/admin/health/refresh',
    {},
    { validateStatus: okish }
  )
  throwIfForbidden(res.status, res.data?.message)
  const body = requireServerSuccess(res.data)
  if (!body.data) {
    throw new Error('health summary missing')
  }
  return body.data
}
