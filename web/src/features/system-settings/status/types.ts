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

export const HEALTH_TARGET_TYPES = [
  'epay',
  'bepusdt',
  'chatwoot',
  'postgres',
  'redis',
  'channel',
  'callback',
] as const

export type HealthTargetType = (typeof HEALTH_TARGET_TYPES)[number]

export type HealthProbeView = {
  target_type: string
  target_id: string
  ok: boolean
  /** A44: backend ok|fail|skipped; absent on older backends. */
  status?: 'ok' | 'fail' | 'skipped' | string
  latency_ms: number
  error: string
  checked_at: string
  last_failure_at?: string | null
}

export type HealthSummary = {
  probes: HealthProbeView[]
  refreshed: boolean
}

export type HealthGetResponse = {
  success: boolean
  message?: string
  data?: HealthSummary
}
