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

import type {
  AdminSite,
  InviteCode,
  InviteCodeInput,
  ResellerSettings,
  ResellerUsage,
  ResellerSettingsUpdate,
  FlaggedCustomer,
  SiteAudit,
  SiteCustomer,
  SitePage,
  SiteReport,
  SiteStatsResponse,
  SiteStatusChange,
  SitesOverview,
} from './types'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

const BASE = '/api/adty/admin'
export const SITE_ADMIN_PAGE_SIZE = 20

export const siteAdminKeys = {
  all: ['adty-site-admin'] as const,
  settings: ['adty-site-admin', 'settings'] as const,
  site: (id: number) => ['adty-site-admin', 'site', id] as const,
  list: (name: string, params: Record<string, unknown>) =>
    ['adty-site-admin', name, params] as const,
}

export async function getSites(params: {
  p: number
  status?: string
  keyword?: string
  page_size?: number
  sort?: string
  order?: 'asc' | 'desc'
}): Promise<SitePage<AdminSite>> {
  const res = await api.get<ApiResponse<SitePage<AdminSite>>>(`${BASE}/sites`, {
    params: { page_size: SITE_ADMIN_PAGE_SIZE, ...params },
  })
  return requireServerSuccess(res.data).data
}

export async function getSitesOverview(): Promise<SitesOverview> {
  const res = await api.get<ApiResponse<SitesOverview>>(
    `${BASE}/sites/overview`
  )
  return requireServerSuccess(res.data).data
}

export async function getSiteStats(id: number): Promise<SiteStatsResponse> {
  const res = await api.get<ApiResponse<SiteStatsResponse>>(
    `${BASE}/sites/${id}/stats`
  )
  return requireServerSuccess(res.data).data
}

export type AdminBalanceRequest = {
  id: number
  site_id: number
  staff_user_id: number
  customer_user_id: number
  direction: 'increase' | 'decrease'
  amount_usd: string
  status: 'pending' | 'approved' | 'rejected'
  reviewer_id: number
  reviewer_role: string
  created_at: number
  reviewed_at: number
}

export async function saveAdminWithdrawRules(body: {
  min_usd: string
  fee_usd: string
  alipay_cny_rate: string
  usdt_usd_rate: string
}): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(
    `${BASE}/site-withdrawals/rules`,
    body
  )
  requireServerSuccess(res.data)
}

export async function issueAdminMarginVoucher(body: {
  site_id: number
  amount_usd: string
  reason: string
}): Promise<{ code: string }> {
  const res = await api.post<ApiResponse<{ code: string }>>(
    `${BASE}/vouchers`,
    { purpose: 'site_margin', ...body }
  )
  return requireServerSuccess(res.data).data
}

export type AdminWallet = {
  id: number
  kind: string
  balance: string
  cycle_tier_usd: string
  cycle_used_usd: string
}

export type AnomalyDisposition = {
  id: number
  kind: string
  ref: string
  status: string
  note: string
  operator_id: number
}

export async function getAdminSiteWallets(siteId: number): Promise<AdminWallet[]> {
  const res = await api.get<ApiResponse<AdminWallet[]>>(
    `${BASE}/sites/${siteId}/wallets`
  )
  return requireServerSuccess(res.data).data ?? []
}

export async function getAdminAnomalyDispositions(
  siteId: number
): Promise<AnomalyDisposition[]> {
  const res = await api.get<ApiResponse<{ items?: AnomalyDisposition[] }>>(
    `${BASE}/fund-anomalies/dispositions`,
    { params: { site_id: siteId } }
  )
  return requireServerSuccess(res.data).data.items ?? []
}

export async function reviewAdminAnomalyDisposition(
  id: number,
  status: 'reviewing' | 'closed',
  note: string
): Promise<void> {
  const res = await api.post<ApiResponse<{ reviewed: boolean }>>(
    `${BASE}/fund-anomalies/dispositions/${id}/review`,
    { status, note }
  )
  requireServerSuccess(res.data)
}

export async function getAdminFundAnomalies(
  siteId: number
): Promise<string[]> {
  const res = await api.get<ApiResponse<{ kinds?: string[] }>>(
    `${BASE}/fund-anomalies`,
    { params: { site_id: siteId } }
  )
  return requireServerSuccess(res.data).data.kinds ?? []
}

export async function getAdminSiteReferralCap(): Promise<{
  cap_configured: boolean
  max_rate?: string
}> {
  const res = await api.get<
    ApiResponse<{ cap_configured: boolean; max_rate?: string }>
  >(`${BASE}/site-referral/cap`)
  return requireServerSuccess(res.data).data
}

export async function saveAdminSiteReferralCap(maxRate: string): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(
    `${BASE}/site-referral/cap`,
    { max_rate: maxRate }
  )
  requireServerSuccess(res.data)
}

export async function getAdminBalanceRequests(
  siteId: number
): Promise<AdminBalanceRequest[]> {
  const res = await api.get<ApiResponse<{ items?: AdminBalanceRequest[] }>>(
    `${BASE}/balance-requests`,
    { params: { site_id: siteId } }
  )
  return requireServerSuccess(res.data).data.items ?? []
}

export async function reviewAdminBalanceRequest(
  id: number,
  approve: boolean
): Promise<void> {
  const res = await api.post<ApiResponse<{ reviewed: boolean }>>(
    `${BASE}/balance-requests/${id}/review`,
    { approve }
  )
  requireServerSuccess(res.data)
}

export async function getFlaggedCustomers(
  p: number
): Promise<SitePage<FlaggedCustomer>> {
  const res = await api.get<ApiResponse<SitePage<FlaggedCustomer>>>(
    `${BASE}/sites/flagged-customers`,
    { params: { p, page_size: SITE_ADMIN_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

export async function getSiteReports(p: number): Promise<SitePage<SiteReport>> {
  const res = await api.get<ApiResponse<SitePage<SiteReport>>>(
    `${BASE}/sites/reports`,
    { params: { p, page_size: SITE_ADMIN_PAGE_SIZE, status: 'pending' } }
  )
  return requireServerSuccess(res.data).data
}

export async function reviewFlaggedCustomer(
  userId: number,
  convertSkipped: boolean
): Promise<{ user_id: number; reviewed: boolean }> {
  const res = await api.post<
    ApiResponse<{ user_id: number; reviewed: boolean }>
  >(`${BASE}/sites/customers/${userId}/review`, {
    convert_skipped: convertSkipped,
  })
  return requireServerSuccess(res.data).data
}

export async function handleSiteReport(
  id: number,
  body: { status: 'resolved' | 'dismissed'; note: string }
): Promise<SiteReport> {
  const res = await api.post<ApiResponse<SiteReport>>(
    `${BASE}/sites/reports/${id}/handle`,
    body
  )
  return requireServerSuccess(res.data).data
}

export type AdminSiteDomain = {
  id: number
  hostname: string
  status: string
}

export type AdminSenderDomain = {
  site_id: number
  domain: string
  status: string
}

export async function getSite(id: number): Promise<{
  site: AdminSite
  audits: SiteAudit[]
  domains: AdminSiteDomain[]
  sender_domains: AdminSenderDomain[]
}> {
  const res = await api.get<
    ApiResponse<{
      site: AdminSite
      audits: SiteAudit[]
      domains: AdminSiteDomain[]
      sender_domains: AdminSenderDomain[]
    }>
  >(`${BASE}/sites/${id}`)
  return requireServerSuccess(res.data).data
}

export async function disableSiteDomain(id: number): Promise<void> {
  const res = await api.delete<ApiResponse<unknown>>(
    `${BASE}/sites/domains/${id}`
  )
  requireServerSuccess(res.data)
}

export async function disableSiteSenderDomain(siteId: number): Promise<void> {
  const res = await api.delete<ApiResponse<unknown>>(
    `${BASE}/sites/${siteId}/sender-domain`
  )
  requireServerSuccess(res.data)
}

export async function getSiteCustomers(
  id: number,
  p: number
): Promise<SitePage<SiteCustomer>> {
  const res = await api.get<ApiResponse<SitePage<SiteCustomer>>>(
    `${BASE}/sites/${id}/customers`,
    { params: { p, page_size: SITE_ADMIN_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

export async function createSite(body: {
  user_id: number
  subdomain: string
  root_domain?: string
  cost_factor?: string
  max_markup?: string
  contact_info?: string
}): Promise<AdminSite> {
  const res = await api.post<ApiResponse<AdminSite>>(`${BASE}/sites`, body)
  return requireServerSuccess(res.data).data
}

export async function reviewSite(
  id: number,
  body: {
    approve: boolean
    cost_factor?: string
    max_markup?: string
    note: string
  }
): Promise<AdminSite> {
  const res = await api.post<ApiResponse<AdminSite>>(
    `${BASE}/sites/${id}/review`,
    body
  )
  return requireServerSuccess(res.data).data
}

export async function adjustSite(
  id: number,
  body: {
    cost_factor?: string
    max_markup?: string
    visibility?: string
    clear?: Array<'cost_factor' | 'max_markup'>
    reason: string
  }
): Promise<AdminSite> {
  const res = await api.put<ApiResponse<AdminSite>>(`${BASE}/sites/${id}`, body)
  return requireServerSuccess(res.data).data
}

export async function resumeSiteSettlement(id: number): Promise<AdminSite> {
  const res = await api.post<ApiResponse<AdminSite>>(
    `${BASE}/sites/${id}/settlement/resume`
  )
  return requireServerSuccess(res.data).data
}

export async function setSiteStatus(
  id: number,
  body: SiteStatusChange
): Promise<AdminSite> {
  const res = await api.put<ApiResponse<AdminSite>>(
    `${BASE}/sites/${id}/status`,
    body
  )
  return requireServerSuccess(res.data).data
}

/** Takes the site's public notice down (site adjust permission); the rest of the brand stays. */
export async function clearSiteNotice(
  id: number,
  reason: string
): Promise<{ site_id: number; cleared: boolean }> {
  const res = await api.delete<
    ApiResponse<{ site_id: number; cleared: boolean }>
  >(`${BASE}/sites/${id}/notice`, { data: { reason } })
  return requireServerSuccess(res.data).data
}

export async function getResellerUsage(): Promise<ResellerUsage> {
  const res = await api.get<ApiResponse<ResellerUsage>>(
    `${BASE}/reseller/usage`
  )
  return requireServerSuccess(res.data).data
}

export async function getResellerSettings(): Promise<ResellerSettings> {
  const res = await api.get<ApiResponse<ResellerSettings>>(
    `${BASE}/reseller/settings`
  )
  return requireServerSuccess(res.data).data
}

export async function saveResellerSettings(
  body: ResellerSettingsUpdate
): Promise<ResellerSettings> {
  const res = await api.put<ApiResponse<ResellerSettings>>(
    `${BASE}/reseller/settings`,
    body
  )
  return requireServerSuccess(res.data).data
}

export async function getInviteCodes(p: number): Promise<SitePage<InviteCode>> {
  const res = await api.get<ApiResponse<SitePage<InviteCode>>>(
    `${BASE}/reseller/invite-codes`,
    { params: { p, page_size: SITE_ADMIN_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

export async function saveInviteCode(
  id: number | null,
  body: InviteCodeInput
): Promise<InviteCode> {
  const res =
    id === null
      ? await api.post<ApiResponse<InviteCode>>(
          `${BASE}/reseller/invite-codes`,
          body
        )
      : await api.put<ApiResponse<InviteCode>>(
          `${BASE}/reseller/invite-codes/${id}`,
          body
        )
  return requireServerSuccess(res.data).data
}

export async function deleteInviteCode(id: number): Promise<void> {
  const res = await api.delete<ApiResponse<unknown>>(
    `${BASE}/reseller/invite-codes/${id}`
  )
  requireServerSuccess(res.data)
}
