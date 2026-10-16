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

import { buildAdminBonusLedgerQuery } from './lib/bonus-ledger'
import type {
  AdminBonusLedgerParams,
  AdminBonusLedgerResponse,
  LedgerEntry,
} from './types'

function throwHttp(status: number, message?: string): never {
  const err = new Error(message || 'request failed') as Error & {
    status?: number
  }
  err.status = status
  throw err
}

export async function listAdminBonusLedger(
  params: AdminBonusLedgerParams
): Promise<{ rows: LedgerEntry[]; total: number }> {
  const query = buildAdminBonusLedgerQuery(params)
  const res = await api.get<AdminBonusLedgerResponse>(
    '/api/admin/wallet/bonus-ledger',
    {
      params: query,
      validateStatus: (s) => (s >= 200 && s < 300) || s === 403,
    }
  )
  if (res.status === 403) {
    throwHttp(403, res.data?.message || 'forbidden')
  }
  const body = requireServerSuccess(res.data)
  return { rows: body.data ?? [], total: body.total ?? 0 }
}
