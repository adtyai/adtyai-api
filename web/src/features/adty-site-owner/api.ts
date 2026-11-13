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
import type { PromoCode, PromoCodeInput } from '@/features/adty-promo/api'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  OwnerCustomer,
  OwnerCustomerDetail,
  OwnerEarnings,
  OwnerLog,
  OwnerPage,
  OwnerSite,
  OwnerTopUp,
  PricingResult,
  SenderDomain,
  SiteBrand,
  SiteDomain,
  SiteStats,
} from './types'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

const BASE = '/api/adty/site'
export const OWNER_PAGE_SIZE = 20

export const siteOwnerKeys = {
  all: ['adty-site-owner'] as const,
  site: ['adty-site-owner', 'site'] as const,
  stats: (from: string, to: string) =>
    ['adty-site-owner', 'stats', from, to] as const,
  list: (name: string, params: Record<string, unknown>) =>
    ['adty-site-owner', name, params] as const,
}

/** Header carrying a step-up proof for sensitive owner actions. */
function proofHeaders(proof: string) {
  return { headers: { 'X-Security-Proof': proof } }
}

export async function requestSiteClose(
  reason: string
): Promise<{ id: number; close_requested_at: number }> {
  const res = await api.post<
    ApiResponse<{ id: number; close_requested_at: number }>
  >(`${BASE}/close`, { reason })
  return requireServerSuccess(res.data).data
}

export async function acceptSiteTerms(version: string): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(`${BASE}/terms/accept`, {
    version,
  })
  requireServerSuccess(res.data)
}

export async function getOwnerSite(): Promise<OwnerSite> {
  const res = await api.get<ApiResponse<OwnerSite>>(`${BASE}/self`)
  return requireServerSuccess(res.data).data
}

/** Brand and the SEO switch do not need verification. */
export async function saveOwnerSite(body: {
  brand?: SiteBrand
  seo_index?: boolean
}): Promise<OwnerSite> {
  const res = await api.put<ApiResponse<OwnerSite>>(`${BASE}/self`, body)
  return requireServerSuccess(res.data).data
}

/** Bonus tiers need a proof for action `bonus` with the site id as target. */
export async function saveBonusRates(
  rates: Record<string, number>,
  proof: string
): Promise<OwnerSite> {
  const res = await api.put<ApiResponse<OwnerSite>>(
    `${BASE}/self`,
    { bonus_rates: rates },
    proofHeaders(proof)
  )
  return requireServerSuccess(res.data).data
}

export async function getOwnerStats(
  from: string,
  to: string
): Promise<SiteStats> {
  const res = await api.get<ApiResponse<SiteStats>>(`${BASE}/stats`, {
    params: { from: from || undefined, to: to || undefined },
  })
  return requireServerSuccess(res.data).data
}

export type SiteWithdrawRules = {
  configured: boolean
  min_usd?: string
  fee_usd?: string
  alipay_cny_rate?: string
  usdt_usd_rate?: string
  methods: string[]
}

export type SiteWithdrawal = {
  id: number
  amount_usd: string
  fee_usd: string
  method: string
  network: string
  status: string
  rate: string
  payout_amount: string
  payout_currency: string
}

export async function getSiteWithdrawRules(): Promise<SiteWithdrawRules> {
  const res = await api.get<ApiResponse<SiteWithdrawRules>>(
    `${BASE}/withdrawals/rules`
  )
  return requireServerSuccess(res.data).data
}

export async function getSiteWithdrawals(): Promise<SiteWithdrawal[]> {
  const res = await api.get<ApiResponse<{ items: SiteWithdrawal[] }>>(
    `${BASE}/withdrawals`
  )
  return requireServerSuccess(res.data).data.items ?? []
}

export async function createSiteWithdrawal(body: {
  amount_usd: string
  method: string
  network: string
  account_no: string
  account_name: string
  expected_rate: string
  idempotencyKey: string
}): Promise<SiteWithdrawal> {
  const res = await api.post<ApiResponse<SiteWithdrawal>>(
    `${BASE}/withdrawals`,
    {
      amount_usd: body.amount_usd,
      method: body.method,
      network: body.network,
      account_no: body.account_no,
      account_name: body.account_name,
      expected_rate: body.expected_rate,
    },
    { headers: { 'Idempotency-Key': body.idempotencyKey } }
  )
  return requireServerSuccess(res.data).data
}

export async function transferEarningToMargin(amountUsd: string): Promise<void> {
  const res = await api.post<ApiResponse<{ transferred: boolean }>>(
    `${BASE}/wallets/earning-to-margin`,
    { amount_usd: amountUsd },
    { headers: { 'Idempotency-Key': crypto.randomUUID() } }
  )
  requireServerSuccess(res.data)
}

export async function cancelSiteWithdrawal(id: number): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/withdrawals/${id}/cancel`
  )
  requireServerSuccess(res.data)
}

export type OwnerEarningLine = {
  id: number
  model_name: string
  group: string
  request_id: string
  earning_usd: string
  created_at: number
}

export async function getOwnerEarningLines(): Promise<OwnerEarningLine[]> {
  const res = await api.get<ApiResponse<OwnerEarningLine[]>>(
    `${BASE}/earnings/lines`,
    { params: { page_size: 20 } }
  )
  return requireServerSuccess(res.data).data ?? []
}

export async function getOwnerEarnings(
  p: number,
  kind: string
): Promise<OwnerEarnings> {
  const res = await api.get<ApiResponse<OwnerEarnings>>(`${BASE}/earnings`, {
    params: { p, page_size: OWNER_PAGE_SIZE, kind: kind || undefined },
  })
  return requireServerSuccess(res.data).data
}

export type SiteReferralView = {
  cap_configured: boolean
  max_rate?: string
  rate_configured: boolean
  rate?: string
  enabled?: boolean
}

export type SiteInvite = {
  id: number
  site_id: number
  inviter_id: number
  invitee_id: number
  rate: string
  created_at: number
}

export type OwnerSiteTeam = {
  id: number
  name: string
  discount: string
  shared_quota: number
  status: string
}

export async function getOwnerSiteTeams(): Promise<OwnerSiteTeam[]> {
  const res = await api.get<ApiResponse<{ items?: OwnerSiteTeam[] }>>(
    `${BASE}/teams`
  )
  return requireServerSuccess(res.data).data.items ?? []
}

export async function saveOwnerSiteTeamDiscount(
  teamId: number,
  discount: string
): Promise<void> {
  const res = await api.put<ApiResponse<{ updated: boolean }>>(
    `${BASE}/teams/${teamId}/discount`,
    { discount }
  )
  requireServerSuccess(res.data)
}

export async function getOwnerSiteReferral(): Promise<SiteReferralView> {
  const res = await api.get<ApiResponse<SiteReferralView>>(`${BASE}/referral`)
  return requireServerSuccess(res.data).data
}

export async function saveOwnerSiteReferralEnabled(
  enabled: boolean
): Promise<void> {
  const res = await api.put<ApiResponse<{ enabled: boolean }>>(
    `${BASE}/referral/enabled`,
    { enabled }
  )
  requireServerSuccess(res.data)
}

export async function saveOwnerSiteReferral(rate: string): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(`${BASE}/referral`, {
    rate,
  })
  requireServerSuccess(res.data)
}

export async function getOwnerSiteInvites(): Promise<SiteInvite[]> {
  const res = await api.get<ApiResponse<{ items?: SiteInvite[] }>>(
    `${BASE}/referral/invites`
  )
  return requireServerSuccess(res.data).data.items ?? []
}

export async function resumeOwnerSiteCoverage(): Promise<void> {
  const res = await api.post<ApiResponse<{ resumed: boolean }>>(
    `${BASE}/referral/cover-pending`
  )
  requireServerSuccess(res.data)
}

export async function bindOwnerSiteInvite(
  inviterId: number,
  inviteeId: number
): Promise<void> {
  const res = await api.post<ApiResponse<{ bound: boolean }>>(
    `${BASE}/referral/invites`,
    { inviter_id: inviterId, invitee_id: inviteeId }
  )
  requireServerSuccess(res.data)
}

export async function getOwnerCustomers(
  p: number
): Promise<OwnerPage<OwnerCustomer>> {
  const res = await api.get<ApiResponse<OwnerPage<OwnerCustomer>>>(
    `${BASE}/customers`,
    { params: { p, page_size: OWNER_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

/** Exact match on the full username or email; rate limited per hour. */
export async function lookupOwnerCustomer(q: string): Promise<OwnerCustomer> {
  const res = await api.get<ApiResponse<OwnerCustomer>>(
    `${BASE}/customers/lookup`,
    { params: { q } }
  )
  return requireServerSuccess(res.data).data
}

export async function getOwnerCustomer(
  id: number
): Promise<OwnerCustomerDetail> {
  const res = await api.get<ApiResponse<OwnerCustomerDetail>>(
    `${BASE}/customers/${id}`
  )
  return requireServerSuccess(res.data).data
}

export async function getOwnerCustomerLogs(
  id: number,
  p: number
): Promise<OwnerPage<OwnerLog>> {
  const res = await api.get<ApiResponse<OwnerPage<OwnerLog>>>(
    `${BASE}/customers/${id}/logs`,
    { params: { p, page_size: OWNER_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

export async function getOwnerCustomerTopUps(
  id: number,
  p: number
): Promise<OwnerPage<OwnerTopUp>> {
  const res = await api.get<ApiResponse<OwnerPage<OwnerTopUp>>>(
    `${BASE}/customers/${id}/topups`,
    { params: { p, page_size: OWNER_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

/** Proof: action `trial`, target = customer id. */
export async function grantTrial(
  id: number,
  amountUsd: string,
  idempotencyKey: string,
  proof: string
): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/customers/${id}/trial`,
    { amount_usd: amountUsd, idempotency_key: idempotencyKey },
    proofHeaders(proof)
  )
  requireServerSuccess(res.data)
}

export async function reportCustomer(
  userId: number,
  reason: string
): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(`${BASE}/report`, {
    user_id: userId,
    reason,
  })
  requireServerSuccess(res.data)
}

/** Proof: action `pricing`, target 0 (site-wide) or the customer id. */
export async function savePricing(
  body: {
    default_markup?: string
    group_markups?: Record<string, string>
    expected_version: number
  },
  proof: string,
  customerId?: number
): Promise<PricingResult> {
  const path = customerId
    ? `${BASE}/customers/${customerId}/pricing`
    : `${BASE}/pricing`
  const res = await api.put<ApiResponse<PricingResult>>(
    path,
    body,
    proofHeaders(proof)
  )
  return requireServerSuccess(res.data).data
}

export async function getOwnerPromoCodes(
  p: number
): Promise<OwnerPage<PromoCode>> {
  const res = await api.get<ApiResponse<OwnerPage<PromoCode>>>(
    `${BASE}/promo-codes`,
    { params: { p, page_size: OWNER_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

/** Proof: action `promo`, target = code id, or 0 when creating. */
export async function saveOwnerPromoCode(
  id: number | null,
  input: PromoCodeInput,
  proof: string
): Promise<PromoCode> {
  const res = id
    ? await api.put<ApiResponse<PromoCode>>(
        `${BASE}/promo-codes/${id}`,
        input,
        proofHeaders(proof)
      )
    : await api.post<ApiResponse<PromoCode>>(
        `${BASE}/promo-codes`,
        input,
        proofHeaders(proof)
      )
  return requireServerSuccess(res.data).data
}

export async function deleteOwnerPromoCode(
  id: number,
  proof: string
): Promise<void> {
  const res = await api.delete<ApiResponse<unknown>>(
    `${BASE}/promo-codes/${id}`,
    proofHeaders(proof)
  )
  requireServerSuccess(res.data)
}

export async function addDomain(hostname: string): Promise<SiteDomain> {
  const res = await api.post<ApiResponse<SiteDomain>>(`${BASE}/domains`, {
    hostname,
  })
  return requireServerSuccess(res.data).data
}

/** Proof: action `domain.delete`, target = domain id. */
export async function removeDomain(id: number, proof: string): Promise<void> {
  const res = await api.delete<ApiResponse<unknown>>(
    `${BASE}/domains/${id}`,
    proofHeaders(proof)
  )
  requireServerSuccess(res.data)
}

/** Uploads a brand image (PNG, JPEG or WebP, up to 200 KB) and returns its address. */
export async function uploadLogo(file: File): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post<ApiResponse<{ url: string }>>(`${BASE}/logo`, form)
  return requireServerSuccess(res.data).data.url
}

/** The site's sender domain, or null when none was added. */
export async function getSenderDomain(): Promise<SenderDomain | null> {
  const res = await api.get<ApiResponse<SenderDomain | null>>(
    `${BASE}/sender-domain`
  )
  return requireServerSuccess(res.data).data
}

export async function addSenderDomain(domain: string): Promise<SenderDomain> {
  const res = await api.post<ApiResponse<SenderDomain>>(
    `${BASE}/sender-domain`,
    { domain }
  )
  return requireServerSuccess(res.data).data
}

/** Proof: action `sender.delete`, target = site id. */
export async function removeSenderDomain(proof: string): Promise<void> {
  const res = await api.delete<ApiResponse<unknown>>(
    `${BASE}/sender-domain`,
    proofHeaders(proof)
  )
  requireServerSuccess(res.data)
}

export type ExportKind = 'earnings' | 'stats' | 'customers'

/** Fetches a CSV export (dashboard auth is a bearer token, so a plain link would not be signed in). */
export async function downloadExport(kind: ExportKind): Promise<void> {
  const res = await api.get<Blob>(`${BASE}/export`, {
    params: { kind },
    responseType: 'blob',
  })
  const url = URL.createObjectURL(res.data)
  const link = document.createElement('a')
  link.href = url
  link.download = `site-${kind}.csv`
  link.click()
  URL.revokeObjectURL(url)
}
