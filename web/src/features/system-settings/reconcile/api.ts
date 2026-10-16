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

import type { ReconcileGetResponse, ReconcileResult } from './types'

function throwIfForbidden(status: number, message?: string): void {
  if (status !== 403) return
  const err = new Error(message || 'forbidden') as Error & { status?: number }
  err.status = 403
  throw err
}

export async function getReconcile(date: string): Promise<ReconcileResult> {
  const res = await api.get<ReconcileGetResponse>('/api/admin/reconcile', {
    params: date ? { date } : undefined,
    validateStatus: (s) => (s >= 200 && s < 300) || s === 403 || s === 400,
  })
  throwIfForbidden(res.status, res.data?.message)
  if (res.status === 400) {
    const err = new Error(res.data?.message || 'bad request', {
      cause: res.data,
    }) as Error & {
      status?: number
    }
    err.status = 400
    throw err
  }
  const body = requireServerSuccess(res.data)
  if (!body.data) {
    throw new Error('reconcile result missing')
  }
  return body.data
}
