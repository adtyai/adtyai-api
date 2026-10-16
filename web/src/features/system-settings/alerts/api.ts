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

import type {
  AlertsConfigView,
  AlertsGetResponse,
  AlertsPutInput,
  AlertsPutResponse,
  AlertsTestInput,
  AlertsTestResponse,
} from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

export async function getAlertsConfig(): Promise<AlertsConfigView> {
  const res = await api.get<AlertsGetResponse>('/api/admin/alerts', {
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
  if (!body.data) {
    throw new Error('alerts config missing')
  }
  return body.data
}

export async function putAlertsConfig(
  input: AlertsPutInput
): Promise<AlertsPutResponse> {
  const res = await api.put<AlertsPutResponse>('/api/admin/alerts', input, {
    validateStatus: okish,
  })
  return res.data
}

export async function testAlerts(
  input: AlertsTestInput
): Promise<AlertsTestResponse> {
  const res = await api.post<AlertsTestResponse>(
    '/api/admin/alerts/test',
    input,
    { validateStatus: okish }
  )
  if (res.data && res.data.success === false) {
    const err = createServerError(res.data) as Error & { code?: string }
    err.code = res.data.code
    throw err
  }
  return requireServerSuccess(res.data)
}
