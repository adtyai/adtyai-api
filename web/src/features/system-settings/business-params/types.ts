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

export type BusinessParamType = 'number' | 'string' | 'json' | 'enum'

export type BusinessParamSnapshot =
  | 'order'
  | 'affiliate'
  | 'withdraw'
  | 'apply'
  | ''

export type BusinessParamRow = {
  key: string
  type: BusinessParamType
  default: string
  min?: number
  max?: number
  enum?: string[]
  snapshot?: BusinessParamSnapshot | string
  page?: string
  notes?: string
  description?: string
  value: string
}

export type BusinessParamsListResponse = {
  success: boolean
  message?: string
  data: BusinessParamRow[]
  meta?: {
    count?: number
    excluded?: string[]
    note?: string
  }
}

export type BusinessParamUpdateItem = {
  key: string
  old_value: string
  new_value: string
  value: string
}

export type BusinessParamPutResponse = {
  success: boolean
  message?: string
  code?: string
  data?: BusinessParamUpdateItem[]
}

export type BusinessParamGroupId =
  | 'topup'
  | 'payment'
  | 'affiliate'
  | 'adjust'
  | 'alerts'
  | 'support'
