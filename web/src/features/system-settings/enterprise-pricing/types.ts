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

export type EnterprisePriceRule = {
  id: number
  user_id: number
  group_code?: string | null
  model_name?: string | null
  discount: number
  expires_at?: string | null
  created_by: number
  created_at?: string
  updated_at?: string
}

export type EnterprisePriceListResponse = {
  success: boolean
  message?: string
  code?: string
  data?: EnterprisePriceRule[]
  total?: number
}

export type EnterprisePriceOneResponse = {
  success: boolean
  message?: string
  code?: string
  data?: EnterprisePriceRule
}

export type EnterprisePriceCreateInput = {
  user_id: number
  group_code?: string | null
  model_name?: string | null
  discount: number
  expires_at?: string | null
  confirm: true
}

export type EnterprisePriceUpdateInput = {
  group_code?: string | null
  model_name?: string | null
  discount?: number
  /** RFC3339; pass "" to clear expiry */
  expires_at?: string | null
  confirm: true
}
