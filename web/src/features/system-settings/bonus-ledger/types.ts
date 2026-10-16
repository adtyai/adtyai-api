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

export type { LedgerEntry } from '@/features/wallet/ledger/types'

export type AdminBonusLedgerParams = {
  user_id?: number
  kind?: string
  created_from?: number | string
  created_to?: number | string
  limit?: number
  offset?: number
}

export type AdminBonusLedgerResponse = {
  success: boolean
  message?: string
  data?: import('@/features/wallet/ledger/types').LedgerEntry[]
  total?: number
}
