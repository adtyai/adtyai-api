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

export type RoutingConfig = {
  enabled: boolean
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

export type ChannelScoreRow = {
  channel_id: number
  channel_name?: string
  model?: string
  score: number | null
  /** Five dimension scores (aliases: s_conn / s_price / … from backend). */
  connectivity?: number | null
  price?: number | null
  speed?: number | null
  stability?: number | null
  authenticity?: number | null
  s_conn?: number | null
  s_price?: number | null
  s_speed?: number | null
  s_stab?: number | null
  s_auth?: number | null
  route_weight?: number | null
  share_pct?: number | null
  sample_30m?: number
  samples?: number
  status_label?: string
  half_open_stage?: HalfOpenStage
  half_open_success?: number
  half_open_inflight?: number
  cooldown_remain_sec?: number
  /** Dual-window detail (5min / 60min); empty until backend ships. */
  conn_short?: number | null
  conn_long?: number | null
  speed_short?: number | null
  speed_long?: number | null
  /** Capability filter reason when excluded from candidates. */
  ability_filter_reason?: string | null
  /** Soft RPM / in-flight throttle. */
  rate_limited?: boolean
  rate_limit_remain_sec?: number | null
  /** Multiplier × recharge rate estimate (CNY / MTok). */
  cost_ratio_rate?: number | null
  /** Last-30-min local estimate (CNY / MTok). */
  cost_live_30m?: number | null
  estimate_sample_ratio?: number | null
  missing_cost?: boolean
  layer?: 'third_party' | 'official'
  fake_model?: boolean
  data_insufficient?: boolean
  no_data?: boolean
  is_new?: boolean
}

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

export type CostRankingRow = {
  channel_id: number
  channel_name?: string
  revenue_cny?: number
  cost_cny?: number
  margin_cny?: number
  margin_pct?: number
}

export type CostGuardRow = {
  channel_id: number
  channel_name?: string
  daily_cost_cny?: number
  daily_avg_7d?: number
  multiplier?: number
  excluded?: boolean
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

export type DecisionRow = {
  request_id?: string
  log_id?: number
  at?: string
  model?: string
  channel_id?: number
  channel_name?: string
  pinned?: boolean
  half_open?: boolean
  tried?: number[]
  exclude_reasons?: string[]
  candidates?: { channel_id: number; route_weight: number }[]
  score_snapshot_at?: string
}

export type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}
