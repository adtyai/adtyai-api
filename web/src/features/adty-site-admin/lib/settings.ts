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
import type {
  CostTier,
  ResellerSettings,
  ResellerSettingsUpdate,
  TurnstileWidget,
} from '../types'

/** Money keys: no built-in value, null until set (convention I). */
export const MONEY_KEYS = [
  'cost_factor',
  'min_cost_factor',
  'max_markup',
  'max_site_bonus_extra',
  'trial_grant_max_usd',
  'trial_grant_daily_usd',
] as const
export type MoneyKey = (typeof MONEY_KEYS)[number]

/** Rules with a starting value, editable. */
export const RULE_KEYS = [
  'markup_increase_delay_hours',
  'settle_hour',
  'freeze_days',
  'close_notice_days',
  'fingerprint_window_days',
  'customer_lookup_per_hour',
  'max_custom_domains_per_site',
  'custom_domain_creates_per_hour',
] as const
export type RuleKey = (typeof RULE_KEYS)[number]

/** Feature switches other than enabled and the two that need Cloudflare / Resend. */
export const SWITCH_KEYS = [
  'site_login_only',
  'invite_code_enabled',
  'self_use_check_enabled',
  'migration_check_enabled',
] as const
export type SwitchKey = (typeof SWITCH_KEYS)[number]

/** Plain Cloudflare fields (the token is a secret and handled separately). */
export const CLOUDFLARE_KEYS = [
  'cloudflare_zone_id',
  'cloudflare_account_id',
  'cname_target',
  'fallback_origin',
] as const
export type CloudflareKey = (typeof CLOUDFLARE_KEYS)[number]

/** Visibility keys and allowed values (task 11.7). */
export const VISIBILITY_KEYS = [
  'customer_email',
  'customer_balance',
  'customer_topups',
  'customer_usage',
  'request_logs',
  'customer_tokens',
] as const
export type VisibilityKey = (typeof VISIBILITY_KEYS)[number]

export type WidgetDraft = TurnstileWidget & {
  /** Stable React key of the row. */
  key: string
  /** New secret; empty keeps the saved one. */
  secret: string
}

export type TierDraft = {
  /** Stable React key of the row. */
  key: string
  min_monthly_usd: string
  cost_factor: string
}

let rowKey = 0
/** A new React key for a tier or widget row. */
export function newRowKey(): string {
  rowKey += 1
  return `row-${rowKey}`
}

export type ResellerDrafts = Record<MoneyKey | RuleKey, string> &
  Record<SwitchKey, boolean> &
  Record<CloudflareKey, string> & {
    enabled: boolean
    custom_domain_enabled: boolean
    sender_domain_enabled: boolean
    unknown_host_mode: 'main' | 'reject'
    /** One per line. */
    root_domains: string
    reserved_subdomains: string
    tiers: TierDraft[]
    visibility: Record<VisibilityKey, string>
    /** New token; empty keeps the saved one. */
    cloudflare_token: string
    clear_cloudflare_token: boolean
    widgets: WidgetDraft[]
  }

const VISIBILITY_DEFAULTS: Record<VisibilityKey, string> = {
  customer_email: 'masked',
  customer_balance: 'on',
  customer_topups: 'on',
  customer_usage: 'on',
  request_logs: 'on',
  customer_tokens: 'on',
}

function lines(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean)
}

export function toResellerDrafts(saved: ResellerSettings): ResellerDrafts {
  const drafts = {
    enabled: saved.enabled,
    custom_domain_enabled: saved.custom_domain_enabled,
    sender_domain_enabled: saved.sender_domain_enabled,
    unknown_host_mode: saved.unknown_host_mode,
    root_domains: (saved.root_domains ?? []).join('\n'),
    reserved_subdomains: (saved.reserved_subdomains ?? []).join('\n'),
    tiers: (saved.cost_factor_tiers ?? []).map((tier) => ({
      key: newRowKey(),
      min_monthly_usd: String(tier.min_monthly_usd),
      cost_factor: String(tier.cost_factor),
    })),
    visibility: { ...VISIBILITY_DEFAULTS, ...saved.visibility },
    cloudflare_token: '',
    clear_cloudflare_token: false,
    widgets: (saved.turnstile_widgets ?? []).map((widget) => ({
      ...widget,
      key: newRowKey(),
      secret: '',
    })),
  } as ResellerDrafts
  for (const key of MONEY_KEYS) {
    drafts[key] = saved[key] === null ? '' : String(saved[key])
  }
  for (const key of RULE_KEYS) drafts[key] = String(saved[key])
  for (const key of SWITCH_KEYS) drafts[key] = saved[key]
  for (const key of CLOUDFLARE_KEYS) drafts[key] = saved[key] ?? ''
  return drafts
}

/** Money keys still unset after these drafts; enabled needs all of them. */
export function missingMoneyKeys(drafts: ResellerDrafts): MoneyKey[] {
  return MONEY_KEYS.filter((key) => drafts[key].trim() === '')
}

/**
 * Whether custom domains can be switched on: the Cloudflare token (saved or
 * typed now), the zone ID and the CNAME target, as the server checks
 * (ResellerSetting.CloudflareConfigured).
 */
export function cloudflareReady(
  saved: ResellerSettings,
  drafts: ResellerDrafts
): boolean {
  const token =
    drafts.cloudflare_token.trim() !== '' ||
    (saved.cloudflare_token_configured && !drafts.clear_cloudflare_token)
  return (
    token &&
    drafts.cloudflare_zone_id.trim() !== '' &&
    drafts.cname_target.trim() !== ''
  )
}

function sameList(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((item, index) => item === b[index])
}

/**
 * PUT body with only the changed keys. An emptied money key is sent as null
 * (back to unset); secrets are only sent when a new value was typed, or
 * listed in clear_secrets.
 */
export function buildResellerUpdate(
  saved: ResellerSettings,
  drafts: ResellerDrafts
): ResellerSettingsUpdate {
  const body: ResellerSettingsUpdate = {}
  for (const key of MONEY_KEYS) {
    const draft = drafts[key].trim()
    const next = draft === '' ? null : Number(draft)
    if (next !== saved[key]) body[key] = next
  }
  for (const key of RULE_KEYS) {
    const next = Number(drafts[key])
    if (next !== saved[key]) body[key] = next
  }
  for (const key of SWITCH_KEYS) {
    if (drafts[key] !== saved[key]) body[key] = drafts[key]
  }
  for (const key of [
    'enabled',
    'custom_domain_enabled',
    'sender_domain_enabled',
  ] as const) {
    if (drafts[key] !== saved[key]) body[key] = drafts[key]
  }
  if (drafts.unknown_host_mode !== saved.unknown_host_mode) {
    body.unknown_host_mode = drafts.unknown_host_mode
  }
  const roots = lines(drafts.root_domains)
  if (!sameList(roots, saved.root_domains ?? [])) body.root_domains = roots
  const reserved = lines(drafts.reserved_subdomains)
  if (!sameList(reserved, saved.reserved_subdomains ?? [])) {
    body.reserved_subdomains = reserved
  }
  const tiers: CostTier[] = drafts.tiers
    .filter((tier) => tier.min_monthly_usd.trim() || tier.cost_factor.trim())
    .map((tier) => ({
      min_monthly_usd: Number(tier.min_monthly_usd),
      cost_factor: Number(tier.cost_factor),
    }))
  if (JSON.stringify(tiers) !== JSON.stringify(saved.cost_factor_tiers ?? [])) {
    body.cost_factor_tiers = tiers
  }
  const savedVisibility = {
    ...VISIBILITY_DEFAULTS,
    ...saved.visibility,
  }
  if (
    VISIBILITY_KEYS.some(
      (key) => drafts.visibility[key] !== savedVisibility[key]
    )
  ) {
    body.visibility = { ...drafts.visibility }
  }
  for (const key of CLOUDFLARE_KEYS) {
    const next = drafts[key].trim()
    if (next !== (saved[key] ?? '')) body[key] = next
  }
  const clear: NonNullable<ResellerSettingsUpdate['clear_secrets']> = []
  if (drafts.cloudflare_token.trim() !== '') {
    body.cloudflare_apiToken = drafts.cloudflare_token.trim()
  } else if (
    drafts.clear_cloudflare_token &&
    saved.cloudflare_token_configured
  ) {
    clear.push('cloudflare_apiToken')
  }
  const widgets: TurnstileWidget[] = drafts.widgets
    .filter((widget) => widget.id.trim() || widget.site_key.trim())
    .map((widget) => ({
      id: widget.id.trim(),
      site_key: widget.site_key.trim(),
    }))
  if (
    JSON.stringify(widgets) !== JSON.stringify(saved.turnstile_widgets ?? [])
  ) {
    body.turnstile_widgets = widgets
  }
  const secrets: Record<string, string> = {}
  for (const widget of drafts.widgets) {
    if (widget.id.trim() && widget.secret.trim()) {
      secrets[widget.id.trim()] = widget.secret.trim()
    }
  }
  if (Object.keys(secrets).length > 0) body.turnstile_widget_secret = secrets
  if (clear.length > 0) body.clear_secrets = clear
  return body
}
