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

export type MenuVisibilitySide = 'user' | 'admin'

export type MenuVisibilityMap = Record<string, boolean>

export type MenuVisibilityView = {
  user: MenuVisibilityMap
  admin: MenuVisibilityMap
}

/** PUT body: non-null side = wholesale replace; omit/null = leave unchanged. */
export type MenuVisibilityPutInput = {
  user?: MenuVisibilityMap | null
  admin?: MenuVisibilityMap | null
}

export type MenuVisibilityGetResponse = {
  success: boolean
  message?: string
  code?: string
  data?: MenuVisibilityView
}

export type MenuVisibilityPutResponse = {
  success: boolean
  message?: string
  code?: string
  data?: MenuVisibilityView
}

export type MenuKeyDef = {
  key: string
  /** i18n key for label */
  labelKey: string
}

export type SideDraft = {
  /** Current on/off for every known + custom key */
  values: MenuVisibilityMap
  /** Extra keys not in the known catalog (from server or user-added) */
  customKeys: string[]
}
