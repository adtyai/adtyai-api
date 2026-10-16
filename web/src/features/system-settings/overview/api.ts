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

import type { AdminOverview, AdminOverviewResponse } from './types'

function throwHttp(status: number, message?: string): never {
  const err = new Error(message || 'request failed') as Error & {
    status?: number
  }
  err.status = status
  throw err
}

export async function getAdminOverview(): Promise<AdminOverview> {
  const res = await api.get<AdminOverviewResponse>('/api/admin/overview', {
    validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
  })
  if (res.status === 403) {
    throwHttp(403, res.data?.message || 'forbidden')
  }
  const body = requireServerSuccess(res.data)
  if (!body.data) {
    throw new Error('overview data missing')
  }
  return body.data
}
