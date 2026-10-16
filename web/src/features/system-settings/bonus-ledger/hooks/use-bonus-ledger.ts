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

import { useQuery } from '@tanstack/react-query'

import { listAdminBonusLedger } from '../api'
import type { AdminBonusLedgerParams } from '../types'

export const ADMIN_BONUS_LEDGER_QUERY_KEY = ['admin-bonus-ledger'] as const

export function useAdminBonusLedger(params: AdminBonusLedgerParams) {
  return useQuery({
    queryKey: [...ADMIN_BONUS_LEDGER_QUERY_KEY, params],
    queryFn: () => listAdminBonusLedger(params),
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}
