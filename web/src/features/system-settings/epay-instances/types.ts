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

export type EpayInstanceStatus = 'enabled' | 'disabled' | string

export type EpayInstanceView = {
  id: number
  name: string
  partner_id: string
  pay_address: string
  callback_base: string
  daily_limit: number
  status: EpayInstanceStatus
  is_primary: boolean
  failure_streak: number
  demoted_reason?: string | null
  demoted_at?: string | null
  last_failure_at?: string | null
  secret_set: boolean
  secret_fingerprint: string
  created_at: string
  updated_at: string
}

export type EpayListResponse = {
  success: boolean
  message?: string
  data?: EpayInstanceView[]
}

export type EpayOneResponse = {
  success: boolean
  message?: string
  code?: string
  data?: EpayInstanceView
}

export type EpayCreateInput = {
  name: string
  partner_id: string
  secret: string
  pay_address: string
  callback_base?: string
  daily_limit?: number
  enabled?: boolean
}

/** Update payload: no enabled - enable/disable only via /enable / /disable + confirm. */
export type EpayUpdateInput = {
  name?: string
  partner_id?: string
  secret?: string
  pay_address?: string
  callback_base?: string
  daily_limit?: number
  confirm?: boolean
}