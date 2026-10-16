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

export type LedgerAccount = 'paid' | 'bonus' | string

/** Mirrors model.LedgerEntry JSON (snake_case). */
export type LedgerEntry = {
  id: number
  user_id: number
  account: LedgerAccount
  kind: string
  delta: number
  balance_after: number
  reason?: string
  ref_type?: string
  ref_id?: string
  hour_bucket?: string | null
  operator_id?: number
  created_at: string
}

export type WalletLedgerSelfParams = {
  account?: '' | 'paid' | 'bonus'
  kind?: string
  created_from?: number | string
  created_to?: number | string
  limit?: number
  offset?: number
}

export type WalletLedgerSelfResponse = {
  success: boolean
  message?: string
  data?: LedgerEntry[]
  total?: number
}
