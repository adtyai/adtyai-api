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

export type GroupMetaView = {
  code: string
  display_name: string
  sort: number
  has_meta: boolean
}

export type GroupMetaCreateInput = {
  code: string
  display_name?: string
  sort?: number
}

/** PUT body — never send a different code (code is immutable). */
export type GroupMetaPutInput = {
  display_name?: string
  sort?: number
}

export type GroupMetaListResponse = {
  success: boolean
  message?: string
  code?: string
  data?: GroupMetaView[]
}

export type GroupMetaOneResponse = {
  success: boolean
  message?: string
  code?: string
  data?: GroupMetaView
}

export type GroupRowDraft = {
  display_name: string
  sort: string
}

export type GroupGrantView = {
  user_id: number
  username: string
  display_name: string
  created_at: number
}

export type GroupGrantListResponse = {
  success: boolean
  message?: string
  code?: string
  data?: GroupGrantView[]
}

export type GroupGrantWriteResponse = {
  success: boolean
  message?: string
  code?: string
}
