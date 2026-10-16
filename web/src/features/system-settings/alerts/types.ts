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

export type AlertsBotTokenMeta = {
  secret_set: boolean
  fingerprint?: string
  option_key: string
  note: string
}

export type AlertsConfigView = {
  bot_token: AlertsBotTokenMeta
  urgent_chat_id: string
  pending_chat_id: string
  urgent_route_enabled: boolean
  pending_route_enabled: boolean
  kind_enabled: Record<string, boolean>
  merge_minutes: number
  thresholds: Record<string, string>
  thresholds_note: string
}

export type AlertsPutInput = {
  bot_token?: string
  urgent_chat_id?: string
  pending_chat_id?: string
  urgent_route_enabled?: boolean
  pending_route_enabled?: boolean
  kind_enabled?: Record<string, boolean>
  merge_minutes?: number
}

export type AlertsGetResponse = {
  success: boolean
  message?: string
  code?: string
  data?: AlertsConfigView
}

export type AlertsPutResponse = {
  success: boolean
  message?: string
  code?: string
  data?: AlertsConfigView
}

export type AlertsTestLevel = 'urgent' | 'pending'

export type AlertsTestInput = {
  level: AlertsTestLevel
  text?: string
}

export type AlertsTestResponse = {
  success: boolean
  message?: string
  code?: string
  data?: {
    level: string
    kind: string
  }
}

export type AlertsFormValues = {
  bot_token: string
  urgent_chat_id: string
  pending_chat_id: string
  urgent_route_enabled: boolean
  pending_route_enabled: boolean
  kind_enabled: Record<string, boolean>
  merge_minutes: number
}
