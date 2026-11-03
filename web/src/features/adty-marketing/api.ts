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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

export const marketingKeys = {
  all: ['adty-marketing'] as const,
  consentDefault: ['adty-marketing', 'consent-default'] as const,
  self: ['adty-marketing', 'self'] as const,
  settings: ['adty-marketing', 'settings'] as const,
  triggers: ['adty-marketing', 'triggers'] as const,
  promoCodes: (page: number) =>
    ['adty-marketing', 'promo-codes', page] as const,
  sources: (params: Record<string, unknown>) =>
    ['adty-marketing', 'sources', params] as const,
  campaigns: (params: Record<string, unknown>) =>
    ['adty-marketing', 'campaigns', params] as const,
  campaign: (id: number) => ['adty-marketing', 'campaign', id] as const,
  estimate: (audience: MarketingAudience) =>
    ['adty-marketing', 'estimate', audience] as const,
}

/** Whether the sign-up page shows the marketing checkbox and ticks it (adty task 12.1). */
export type MarketingConsentDefault = { show: boolean; checked: boolean }

export async function getMarketingConsentDefault(): Promise<MarketingConsentDefault> {
  const res = await api.get<ApiResponse<MarketingConsentDefault>>(
    '/api/adty/marketing/consent-default'
  )
  return requireServerSuccess(res.data).data
}

export type MarketingSelf = {
  consent: boolean
  consent_at: number
  blocked: boolean
  enabled: boolean
}

export async function getMarketingSelf(): Promise<MarketingSelf> {
  const res = await api.get<ApiResponse<MarketingSelf>>(
    '/api/adty/marketing/self'
  )
  return requireServerSuccess(res.data).data
}

export async function saveMarketingSelf(body: {
  consent: boolean
  lang: string
}): Promise<MarketingSelf> {
  const res = await api.put<ApiResponse<MarketingSelf>>(
    '/api/adty/marketing/self',
    body
  )
  return requireServerSuccess(res.data).data
}

/** Admin marketing settings; the webhook secret is never returned. */
export type MarketingSettings = {
  enabled: boolean
  from_address: string
  resend_segment_prefix: string
  consent_default_non_eu: boolean
  consent_version: string
  eu_countries: string
  footer_company: string
  footer_address: string
  open_tracking: boolean
  click_tracking: boolean
  sync_batch_size: number
  sync_interval_seconds: number
  api_rate_per_second: number
  include_site_customers_default: boolean
  promo_enabled: boolean
  webhook_secret_configured: boolean
  resend_key_configured: boolean
  missing: string[]
}

export type MarketingSettingsUpdate = Partial<
  Omit<
    MarketingSettings,
    'webhook_secret_configured' | 'resend_key_configured' | 'missing'
  >
> & { webhook_secret?: string; clear_webhook_secret?: boolean }

export async function getMarketingSettings(): Promise<MarketingSettings> {
  const res = await api.get<ApiResponse<MarketingSettings>>(
    '/api/adty/admin/marketing/settings'
  )
  return requireServerSuccess(res.data).data
}

export async function saveMarketingSettings(
  body: MarketingSettingsUpdate
): Promise<MarketingSettings> {
  const res = await api.put<ApiResponse<MarketingSettings>>(
    '/api/adty/admin/marketing/settings',
    body
  )
  return requireServerSuccess(res.data).data
}

/** Root only. The secret is write-only and is not returned. */
export async function saveMarketingSecrets(body: {
  webhook_secret?: string
  clear_webhook_secret?: boolean
}): Promise<MarketingSettings> {
  const res = await api.put<ApiResponse<MarketingSettings>>(
    '/api/adty/admin/marketing/secrets',
    body
  )
  return requireServerSuccess(res.data).data
}

export type ContentTexts = Partial<Record<string, string>>

/** Recipient filters (adty task 12.2); empty values mean no limit. */
export type MarketingAudience = {
  langs?: string[]
  registered_after?: number
  registered_before?: number
  active_after?: number
  active_before?: number
  topped_up?: boolean | null
  balance_min_usd?: string
  balance_max_usd?: string
  groups?: string[]
  include_site_customers?: boolean
  site_ids?: number[]
}

export type CampaignStatus =
  | 'draft'
  | 'preparing'
  | 'scheduled'
  | 'sent'
  | 'canceled'
  | 'failed'

export type MarketingCampaign = {
  id: number
  name: string
  is_template: boolean
  status: CampaignStatus
  subjects: ContentTexts
  bodies: ContentTexts
  audience: MarketingAudience
  scheduled_at: number
  recipients: number
  error: string
  created_at: number
  sent_at: number
}

export type CampaignInput = Pick<
  MarketingCampaign,
  'name' | 'is_template' | 'subjects' | 'bodies' | 'audience' | 'scheduled_at'
>

export type CampaignStats = {
  recipients: number
  delivered: number
  bounced: number
  complained: number
  unsubscribed: number
  opened: number
  clicked: number
}

export type CampaignSend = {
  id: number
  site_id: number
  lang: string
  recipients: number
  status: string
  broadcast_id: string
}

export type CampaignDetail = {
  campaign: MarketingCampaign
  stats: CampaignStats
  sends: CampaignSend[]
}

export type AudienceEstimate = {
  total: number
  by_lang: Record<string, number>
}
export type CampaignPlan = {
  total: number
  versions: Record<string, number>
  sites: number
}

export type Page<T> = { items: T[] | null; total: number; page: number }

const ADMIN = '/api/adty/admin/marketing'

export async function getCampaigns(params: {
  p: number
  is_template: boolean
}): Promise<Page<MarketingCampaign>> {
  const res = await api.get<ApiResponse<Page<MarketingCampaign>>>(
    `${ADMIN}/campaigns`,
    { params: { ...params, page_size: 20 } }
  )
  return requireServerSuccess(res.data).data
}

export async function getCampaign(id: number): Promise<CampaignDetail> {
  const res = await api.get<ApiResponse<CampaignDetail>>(
    `${ADMIN}/campaigns/${id}`
  )
  return requireServerSuccess(res.data).data
}

export async function estimateAudience(
  audience: MarketingAudience
): Promise<AudienceEstimate> {
  const res = await api.post<ApiResponse<AudienceEstimate>>(
    `${ADMIN}/audience/estimate`,
    audience
  )
  return requireServerSuccess(res.data).data
}

export async function saveCampaign(
  id: number | null,
  input: CampaignInput
): Promise<MarketingCampaign> {
  const res = id
    ? await api.put<ApiResponse<MarketingCampaign>>(
        `${ADMIN}/campaigns/${id}`,
        input
      )
    : await api.post<ApiResponse<MarketingCampaign>>(
        `${ADMIN}/campaigns`,
        input
      )
  return requireServerSuccess(res.data).data
}

export async function deleteCampaign(id: number): Promise<void> {
  const res = await api.delete<ApiResponse<null>>(`${ADMIN}/campaigns/${id}`)
  requireServerSuccess(res.data)
}

export async function getCampaignPlan(id: number): Promise<CampaignPlan> {
  const res = await api.get<ApiResponse<CampaignPlan>>(
    `${ADMIN}/campaigns/${id}/plan`
  )
  return requireServerSuccess(res.data).data
}

export async function sendCampaignTest(
  id: number,
  body: { to: string; lang: string }
): Promise<void> {
  const res = await api.post<ApiResponse<null>>(
    `${ADMIN}/campaigns/${id}/test`,
    body
  )
  requireServerSuccess(res.data)
}

export async function sendCampaign(
  id: number,
  recipients: number
): Promise<CampaignPlan> {
  const res = await api.post<ApiResponse<CampaignPlan>>(
    `${ADMIN}/campaigns/${id}/send`,
    { confirm: true, recipients }
  )
  return requireServerSuccess(res.data).data
}

export async function cancelCampaign(id: number): Promise<void> {
  const res = await api.post<ApiResponse<null>>(
    `${ADMIN}/campaigns/${id}/cancel`,
    {}
  )
  requireServerSuccess(res.data)
}

/** One automated email rule (adty task 12.3). `days` or `usd` depends on the rule. */
export type TriggerRule = {
  enabled: boolean
  days?: number
  usd?: string
  subjects: ContentTexts
  bodies: ContentTexts
}

export type TriggerKey = 'unpaid' | 'low_balance' | 'winback'

export type MarketingTriggers = Record<TriggerKey, TriggerRule> & {
  batch_size: number
}

export async function getMarketingTriggers(): Promise<MarketingTriggers> {
  const res = await api.get<ApiResponse<MarketingTriggers>>(`${ADMIN}/triggers`)
  return requireServerSuccess(res.data).data
}

export async function saveMarketingTriggers(
  body: Partial<MarketingTriggers>
): Promise<MarketingTriggers> {
  const res = await api.put<ApiResponse<MarketingTriggers>>(
    `${ADMIN}/triggers`,
    body
  )
  return requireServerSuccess(res.data).data
}

export type SignupSourceDimension =
  | 'utm_source'
  | 'utm_medium'
  | 'utm_campaign'
  | 'referrer_host'
  | 'landing_path'
  | 'inviter'
  | 'site'

/** One row of sign-up source statistics (adty task 12.5); money is USD strings. */
export type SignupSourceStat = {
  key: string
  signups: number
  paying: number
  topup_usd: string
  consume_usd: string
}

export async function getSignupSources(params: {
  dimension: SignupSourceDimension
  from: number
  to: number
}): Promise<{ dimension: string; items: SignupSourceStat[] | null }> {
  const res = await api.get<
    ApiResponse<{ dimension: string; items: SignupSourceStat[] | null }>
  >(`${ADMIN}/sources`, { params })
  return requireServerSuccess(res.data).data
}

/** Promo code (adty task 12.4). Money and percent values are decimal strings. */
export type PromoCode = {
  id: number
  code: string
  kind: 'percent' | 'fixed'
  value: string
  min_face_usd: string
  max_uses: number
  per_user_limit: number
  used_count: number
  starts_at: number
  expires_at: number
  new_users_only: boolean
  site_id: number
  funder: 'platform' | 'site_owner'
  enabled: boolean
  note: string
}

export type PromoCodeInput = Omit<PromoCode, 'id' | 'used_count' | 'funder'>

export async function getPromoCodes(page: number): Promise<Page<PromoCode>> {
  const res = await api.get<ApiResponse<Page<PromoCode>>>(
    `${ADMIN}/promo-codes`,
    {
      params: { p: page, page_size: 20 },
    }
  )
  return requireServerSuccess(res.data).data
}

export async function savePromoCode(
  id: number | null,
  input: PromoCodeInput
): Promise<PromoCode> {
  const res = id
    ? await api.put<ApiResponse<PromoCode>>(`${ADMIN}/promo-codes/${id}`, input)
    : await api.post<ApiResponse<PromoCode>>(`${ADMIN}/promo-codes`, input)
  return requireServerSuccess(res.data).data
}
