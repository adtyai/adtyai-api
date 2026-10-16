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

import type { ScoreWeights } from './types'

/** Factory default "balanced" weights. */
export const DEFAULT_SCORE_WEIGHTS: ScoreWeights = {
  connectivity: 30,
  price: 25,
  speed: 20,
  stability: 15,
  authenticity: 10,
}

export const DEFAULT_ROUTING_CONFIG = {
  enabled: false,
  weights: DEFAULT_SCORE_WEIGHTS,
  half_open_inflight_max: 2,
  hysteresis_recover: 65,
  breaker_enter: 40,
  floor_total_pct: 30,
  retry_budget_pct: 20,
  session_sticky: false,
  epsilon_greedy: false,
  peer_breaker: false,
  telegram_alert: false,
  region_filter: false,
} as const

export const WEIGHT_DIM_KEYS = [
  'connectivity',
  'price',
  'speed',
  'stability',
  'authenticity',
] as const

export const WEIGHT_DIM_LABEL_KEYS: Record<
  (typeof WEIGHT_DIM_KEYS)[number],
  string
> = {
  connectivity: 'Connectivity',
  price: 'Price',
  speed: 'Speed',
  stability: 'Stability',
  authenticity: 'Authenticity',
}

export const HALF_OPEN_SUCCESS_TARGET = 5
export const HALF_OPEN_INFLIGHT_MAX = 2

/** API prefix for smart routing admin endpoints. */
export const SMART_ROUTING_API_PREFIX = '/api/adty/routing'
