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
/** Server-paged list (common.PageInfo). */
export type OwnerPage<T> = {
  page: number
  page_size: number
  total: number
  items: T[] | null
}

export type OwnerSiteStatus = 'active' | 'paused' | 'closing'

/** Brand fields the owner may edit; rendered as plain text everywhere. */
export type SiteBrand = {
  name: string
  logo_url: string
  title?: string
  favicon_url?: string
  theme_color?: string
  tagline: string
  notice: string
  support_contact: string
}

/** One site price row (model.AdtySitePrice). Decimals are strings. */
export type SitePrice = {
  id: number
  site_id: number
  user_id: number
  group: string
  markup: string
  pending_markup: string
  pending_at: number
  version: number
}

/** A custom domain (model.AdtySiteDomain, task 11.8). */
export type SiteDomain = {
  id: number
  hostname: string
  status: 'pending' | 'active' | 'failed' | 'removed'
  ssl_status: string
  verify_records: string
  last_error: string
  created_at: number
}

/** GET /api/adty/site/self (controller/adty_site_owner.go). */
export type OwnerSite = {
  id: number
  status: OwnerSiteStatus
  subdomain: string
  root_domain: string
  host: string
  base_url: string
  cost_factor: string
  markup_min: string
  markup_max: string
  markup_increase_delay_hours: number
  custom_domain_enabled: boolean
  sender_domain_enabled: boolean
  brand: SiteBrand
  /** Tier USD (as a JSON object key) → bonus percent. */
  bonus_rates: Record<string, number>
  seo_index: boolean
  agreement_version: string
  agreement_at: number
  close_requested_at?: number
  prices: SitePrice[] | null
  pricing_version: number
  domains: SiteDomain[] | null
  visibility: Record<string, string>
}

/** One day (or the total) of GET /api/adty/site/stats. Money fields are USD decimal strings. */
export type SiteDayStats = {
  day: string
  new_customers: number
  paid_customers: number
  active_customers: number
  topup_usd: string
  revenue_usd: string
  platform_usd: string
  earning_usd: string
  bonus_usd: string
  refund_usd: string
  skipped_usd: string
}

export type SiteStats = {
  from: string
  to: string
  total: SiteDayStats
  series: SiteDayStats[] | null
}

export type EarningKind =
  | 'usage'
  | 'refund'
  | 'bonus'
  | 'bonus_reversal'
  | 'trial'
  | 'skipped'

export type EarningRow = {
  day: string
  user_id: number
  kind: EarningKind
  reason?: 'self_use' | 'migration'
  revenue_usd: string
  earning_usd: string
  status: 'pending' | 'available' | 'void'
  settled_at: number
}

export type SiteBalances = {
  pending_usd: string
  skipped_usd: string
  available_usd: string
  frozen_usd: string
  withdrawn_usd: string
}

export type OwnerEarnings = {
  balances: SiteBalances
  page: OwnerPage<EarningRow>
}

/** A customer as the owner sees it: email masked unless the platform allows full. */
export type OwnerCustomer = {
  id: number
  username: string
  email: string
  status: number
  group: string
  created_at: number
  balance?: number
  used_quota?: number
  risk_flag?: 'self_use' | 'migration'
  reviewed: boolean
}

export type OwnerToken = {
  id: number
  name: string
  status: number
  accessed_time: number
  created_time: number
}

export type OwnerCustomerDetail = {
  customer: OwnerCustomer
  tokens?: OwnerToken[]
  /** This customer's own prices (user_id = customer). */
  prices: SitePrice[] | null
  pricing_version: number
}

export type OwnerLog = {
  id: number
  created_at: number
  type: number
  model_name: string
  token_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  use_time: number
  is_stream: boolean
}

export type OwnerTopUp = {
  trade_no: string
  face_usd: string
  bonus_usd: string
  credit_quota: number
  method: string
  status: string
  created_at: number
}

/** One DNS record Resend asks for (service.AdtyResendDomainRecord). */
export type SenderDnsRecord = {
  record: string
  name: string
  type: string
  value: string
  ttl: string
  status: string
  priority?: number
}

/** GET /api/adty/site/sender-domain (model.AdtySiteSenderDomain, task 11.3). */
export type SenderDomain = {
  site_id: number
  domain: string
  status: 'pending' | 'verified' | 'failed' | 'disabled'
  /** JSON array of SenderDnsRecord. */
  records: string
  disabled_by: number
  last_checked_at: number
  created_at: number
}

/** Result of PUT /api/adty/site/pricing. */
export type PricingResult = {
  version: number
  immediate: string[] | null
  /** Group ('' is the default) → time the increase takes effect. */
  pending: Record<string, number> | null
}
