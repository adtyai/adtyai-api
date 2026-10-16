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
  EpayCreateInput,
  EpayInstanceView,
  EpayListResponse,
  EpayOneResponse,
  EpayUpdateInput,
} from './types'

const okish = (s: number) =>
  (s >= 200 && s < 300) || s === 400 || s === 403 || s === 404 || s === 409

export async function listEpayInstances(): Promise<EpayInstanceView[]> {
  const res = await api.get<EpayListResponse>('/api/admin/epay-instances', {
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

export async function createEpayInstance(
  input: EpayCreateInput
): Promise<EpayOneResponse> {
  const res = await api.post<EpayOneResponse>(
    '/api/admin/epay-instances',
    input,
    { validateStatus: okish }
  )
  return res.data
}

export async function updateEpayInstance(
  id: number,
  input: EpayUpdateInput
): Promise<EpayOneResponse> {
  const res = await api.put<EpayOneResponse>(
    `/api/admin/epay-instances/${  id}`,
    input,
    { validateStatus: okish }
  )
  return res.data
}

export async function setEpayInstancePrimary(
  id: number
): Promise<EpayOneResponse> {
  const res = await api.post<EpayOneResponse>(
    `/api/admin/epay-instances/${  id  }/set-primary`,
    { confirm: true },
    { validateStatus: okish }
  )
  return res.data
}

export async function disableEpayInstance(
  id: number
): Promise<EpayOneResponse> {
  const res = await api.post<EpayOneResponse>(
    `/api/admin/epay-instances/${  id  }/disable`,
    { confirm: true },
    { validateStatus: okish }
  )
  return res.data
}

export async function enableEpayInstance(id: number): Promise<EpayOneResponse> {
  const res = await api.post<EpayOneResponse>(
    `/api/admin/epay-instances/${  id  }/enable`,
    { confirm: true },
    { validateStatus: okish }
  )
  return res.data
}
