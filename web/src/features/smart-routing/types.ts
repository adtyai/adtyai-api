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

/** Five scoring dimensions; defaults 30/25/20/15/10 (balanced). */
export type ScoreWeights = {
  connectivity: number
  price: number
  speed: number
  stability: number
  authenticity: number
}

export type WeightScheme = {
  id: string
  name: string
  weights: ScoreWeights
  note?: string
  created_at?: string
  created_by?: string
  is_default?: boolean
}

export type RouteTagRow = {
  id: number
  channel_id: number
  model: string
  tag: string
  note?: string
}

export type RoutingConfig = {
  enabled: boolean
  /** Server mode. enabled is true only when mode is active. */
  mode?: 'off' | 'observe' | 'active'
  /** Backend field: active scheme name (e.g. balanced_default). */
  active_weight_scheme?: string
  /** @deprecated FE alias; prefer active_weight_scheme. */
  active_weight_scheme_id?: string
  weights: ScoreWeights
  /** Soft caps / breaker params (read/write for root). */
  half_open_inflight_max?: number
  hysteresis_recover?: number
  breaker_enter?: number
  floor_total_pct?: number
  retry_budget_pct?: number
  /** Default-off optional switches. */
  session_sticky?: boolean
  epsilon_greedy?: boolean
  peer_breaker?: boolean
  telegram_alert?: boolean
  region_filter?: boolean
}

export type HalfOpenStage =
  | 'closed'
  | 'open'
  | 'probe_2'
  | 'ramp_10'
  | 'full'
  | 'unknown'

export type BreakerStageRow = {
  channel_id: number
  channel_name?: string
  stage: HalfOpenStage
  success_count?: number
  inflight?: number
  cooldown_remain_sec?: number
  updated_at?: string
}

export type BreakerEvent = {
  id?: string
  channel_id: number
  channel_name?: string
  event: string
  at: string
  detail?: string
}

export type ChannelPriceRow = {
  channel_id: number
  channel_name?: string
  /** 1 CNY = N upstream USD credits. */
  recharge_credits_per_cny?: number | null
  /** Reused NewAPI channel ratio / multiplier. */
  channel_ratio?: number | null
  /** Advanced absolute USD per 1M tokens (optional). */
  absolute_usd_per_mtok?: number | null
  absolute_enabled?: boolean
  cost_ratio_rate?: number | null
  cost_live_30m?: number | null
  estimate_sample_ratio?: number | null
  missing_cost?: boolean
  guard_excluded?: boolean
  daily_cost_cny?: number | null
  daily_avg_7d?: number | null
}

export type SimulateResultItem = {
  channel_id: number
  channel_name?: string
  route_weight: number
  share_pct: number
  half_open?: boolean
  stage?: HalfOpenStage
  score?: number | null
}

export type SimulateResult = {
  model: string
  items: SimulateResultItem[]
  error?: string
  picked?: number
  would_pick?: number
  half_open?: {
    channel_id: number
    priority?: number
    share?: number
    band_index?: number
  }
}

export type WeightPreviewItem = {
  channel_id: number
  channel_name?: string
  model?: string
  /** Mapped from backend rw_now (route weight). */
  share_now: number
  /** Mapped from backend rw_new (route weight). */
  share_new: number
  score_now?: number
  score_new?: number
}

export type WeightPreview = {
  items: WeightPreviewItem[]
  /** Server gate token from POST /weight-schemes/preview; required on Confirm PUT. */
  preview_token?: string
  /** Unix seconds expiry from backend. */
  expires_at?: number
  intent?: Record<string, unknown>
  /** Set when the preview request failed (F-P2-2). */
  error?: string
  error_code?: string
}

export type WeightHistoryEntry = {
  id: string
  at: string
  by?: string
  summary?: string
  scheme_name?: string
  /** Raw snapshot JSON (config) when backend returns it; used for rollback preview. */
  snapshot?: string | Record<string, unknown>
}

export type ApiEnvelope<T> = {
  success: boolean
  message?: string
  code?: string
  /** HTTP status when the error body was not a JSON envelope. */
  status?: number
  data?: T
}
