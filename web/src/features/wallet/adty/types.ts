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
export type AdtyMethodKind = 'usdt' | 'epay' | 'hosted'

export type AdtyMethod = {
  id: string
  kind: AdtyMethodKind
  available: boolean
  label?: string
  /** Fixed product face value in USD. */
  face_usd?: number | string
  currency?: string
}

export type AdtyPaymentScene = {
  key: string
  name: string
  min_usd: string
  max_usd: string
  methods: AdtyMethod[]
}

export type AdtyTier = { amount: number; rate: number }

/** Our order statuses; upstream TopupStatus only has three. */
export type AdtyOrderStatus =
  | 'pending'
  | 'success'
  | 'expired'
  | 'canceled'
  | 'failed'
  | 'refunded'

export type AdtyOrderExt = {
  payment_scene?: string
  payment_scene_name?: string
  payment_scene_version?: number
  trade_no: string
  provider: string
  method: string
  face_usd: string
  bonus_rate: string
  bonus_usd: string
  /** Extra promo credit already included in credit_quota. */
  promo_bonus_usd: string
  credit_quota: number
  pay_amount: string
  pay_currency: string
  price_snapshot: string
  expire_at: number
  gateway_trade_id: string
  pay_address: string
  pay_url: string
  qr_code: string
  url_scheme: string
  channel_id: number
  paid_amount: string
  paid_currency: string
  paid_tx_hash: string
  refunded_at: number
}

export type AdtyOrder = {
  id: number
  user_id: number
  trade_no: string
  payment_method: string
  payment_provider: string
  status: AdtyOrderStatus
  create_time: number
  complete_time: number
  amount: number
  money: number
  ext: AdtyOrderExt | null
}

export type AdtyTopupInfo = {
  scenes?: AdtyPaymentScene[]
  tiers: AdtyTier[]
  bonus_rates: Record<string, number>
  min_topup: number
  methods: AdtyMethod[]
  price_cny_per_usd: number
  usdt_order_minutes: number
  epay_order_minutes: number
  pending: AdtyOrder | null
  compliance_confirmed: boolean
}

export type AdtyQuote = {
  face_usd: string
  bonus_rate: string
  bonus_usd: string
  credit_quota: number
  pay_amount: string
  pay_currency: string
  price_cny_per_usd: string
  topup_group_ratio: string
  /** adty task 12.4: extra credit from a promo code, already in credit_quota */
  promo_code_id: number
  promo_bonus_usd: string
}

export type AdtyPage<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}
