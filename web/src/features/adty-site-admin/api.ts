import type { SiteWalletLedgerPage } from '@/features/adty-ledger/site-wallet-labels'
import type {
  VoucherIssued,
  VoucherPage,
} from '@/features/adty-ledger/voucher-labels'
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
import type { SettlementSheet } from '@/features/adty-site-owner/api'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  AdminSite,
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

/**
 * 服务端把空集合序列化成 null 或对象时，`data ?? []` 会把这个非数组原样返回，
 * 调用方一旦 `.map` 就整块白屏。列表只有数组才算数，其它形状一律按空列表处理。
 */
function listOrEmpty<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}

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
  frozen_usd?: string
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
  site_id?: number
  account_id?: number
  order_ref?: string
  difference?: string
  evidence?: string
  created_at?: number
}

export async function getAdminSiteWallets(
  siteId: number
): Promise<AdminWallet[]> {
  const res = await api.get<ApiResponse<AdminWallet[]>>(
    `${BASE}/sites/${siteId}/wallets`
  )
  return listOrEmpty(requireServerSuccess(res.data).data)
}

/** Root only: every wallet kind of one site, newest first. */
export async function getAdminSiteWalletLedger(
  siteId: number,
  kind: string,
  page: number,
  pageSize: number
): Promise<SiteWalletLedgerPage> {
  const res = await api.get<ApiResponse<SiteWalletLedgerPage>>(
    `${BASE}/sites/${siteId}/wallet-ledgers`,
    { params: { kind: kind || undefined, p: page, page_size: pageSize } }
  )
  return requireServerSuccess(res.data).data
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

export async function getAdminFundAnomalies(siteId: number): Promise<string[]> {
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

export async function getAdminBusinessTimezone(): Promise<{
  configured: boolean
  timezone: string
}> {
  const res = await api.get<
    ApiResponse<{ configured: boolean; timezone: string }>
  >(`${BASE}/business-timezone`)
  return requireServerSuccess(res.data).data
}

export async function saveAdminBusinessTimezone(
  timezone: string
): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(
    `${BASE}/business-timezone`,
    { timezone }
  )
  requireServerSuccess(res.data)
}

export async function getAdminSettlementSheets(
  siteId?: number
): Promise<SettlementSheet[]> {
  const res = await api.get<ApiResponse<SettlementSheet[]>>(
    `${BASE}/settlement-sheets`,
    { params: siteId ? { site_id: siteId } : undefined }
  )
  return listOrEmpty(requireServerSuccess(res.data).data)
}

export async function getAdminSiteInviteBatchPolicy(): Promise<{
  configured: boolean
  threshold?: string
  window_hours?: string
}> {
  const res = await api.get<
    ApiResponse<{
      configured: boolean
      threshold?: string
      window_hours?: string
    }>
  >(`${BASE}/site-referral/batch-policy`)
  return requireServerSuccess(res.data).data
}

export async function saveAdminSiteInviteBatchPolicy(
  threshold: string,
  windowHours: string
): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(
    `${BASE}/site-referral/batch-policy`,
    { threshold, window_hours: windowHours }
  )
  requireServerSuccess(res.data)
}

export async function getAdminSiteCommissionPolicy(): Promise<{
  configured: boolean
  pending_coverage_cap?: string
  observation_days?: string
}> {
  const res = await api.get<
    ApiResponse<{
      configured: boolean
      pending_coverage_cap?: string
      observation_days?: string
    }>
  >(`${BASE}/site-referral/commission-policy`)
  return requireServerSuccess(res.data).data
}

export async function getAdminSiteInviteWithdrawRules(): Promise<{
  configured: boolean
  min_usd?: string
  fee_usd?: string
  cny_rate?: string
}> {
  const res = await api.get<
    ApiResponse<{
      configured: boolean
      min_usd?: string
      fee_usd?: string
      cny_rate?: string
    }>
  >(`${BASE}/site-invite-withdraw/rules`)
  return requireServerSuccess(res.data).data
}

export async function saveAdminSiteInviteWithdrawRules(
  minUsd: string,
  feeUsd: string,
  cnyRate: string
): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(
    `${BASE}/site-invite-withdraw/rules`,
    { min_usd: minUsd, fee_usd: feeUsd, cny_rate: cnyRate }
  )
  requireServerSuccess(res.data)
}

export async function saveAdminSiteCommissionPolicy(
  pendingCoverageCap: string,
  observationDays: string
): Promise<void> {
  const res = await api.put<ApiResponse<{ saved: boolean }>>(
    `${BASE}/site-referral/commission-policy`,
    {
      pending_coverage_cap: pendingCoverageCap,
      observation_days: observationDays,
    }
  )
  requireServerSuccess(res.data)
}

export async function getAdminSiteCommissionMargin(
  siteId: number
): Promise<{ allowed: boolean }> {
  const res = await api.get<ApiResponse<{ site_id: number; allowed: boolean }>>(
    `${BASE}/sites/${siteId}/commission-margin`
  )
  return requireServerSuccess(res.data).data
}

export async function saveAdminSiteCommissionMargin(
  siteId: number,
  allowed: boolean
): Promise<void> {
  const res = await api.put<ApiResponse<{ site_id: number; allowed: boolean }>>(
    `${BASE}/sites/${siteId}/commission-margin`,
    { allowed }
  )
  requireServerSuccess(res.data)
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
    allowed_groups?: string
    clear?: Array<'cost_factor' | 'max_markup' | 'allowed_groups'>
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

export async function getPausedSiteRepairs(): Promise<AdminSite[]> {
  const res = await api.get<ApiResponse<AdminSite[]>>(
    `${BASE}/sites/paused-repairs`
  )
  return listOrEmpty(requireServerSuccess(res.data).data)
}

export async function repairPausedSite(
  id: number,
  body: { outcome: 'active' | 'close'; reason: string; immediate?: boolean }
): Promise<AdminSite> {
  const res = await api.post<ApiResponse<AdminSite>>(
    `${BASE}/sites/${id}/paused-repair`,
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

/** One site earnings withdrawal in Root's review list. Amounts are USD. */
export type AdminSiteWithdrawal = {
  id: number
  site_id: number
  owner_user_id: number
  subdomain: string
  amount_usd: string
  fee_usd: string
  net_usd: string
  method: string
  network: string
  account_no: string
  account_name: string
  rate: string
  payout_amount: string
  payout_currency: string
  status: string
  tx_id: string
  proof_note: string
  reviewer_id: number
  reviewed_at: number
  paid_at: number
  created_at: number
}

export async function getAdminSiteWithdrawals(
  status: string,
  page: number,
  pageSize: number
): Promise<{ total: number; items: AdminSiteWithdrawal[] }> {
  const res = await api.get<
    ApiResponse<{ total: number; items: AdminSiteWithdrawal[] }>
  >(`${BASE}/site-withdrawals`, {
    params: { status: status || undefined, p: page, page_size: pageSize },
  })
  return requireServerSuccess(res.data).data
}

export async function markSiteWithdrawalPaying(id: number): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/site-withdrawals/${id}/paying`
  )
  requireServerSuccess(res.data)
}

export async function markSiteWithdrawalPaid(
  id: number,
  body: { tx_id: string; proof: string }
): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/site-withdrawals/${id}/paid`,
    body
  )
  requireServerSuccess(res.data)
}

export async function rejectSiteWithdrawal(
  id: number,
  reason: string
): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/site-withdrawals/${id}/reject`,
    { reason }
  )
  requireServerSuccess(res.data)
}

export type VoucherIssuance = {
  limit_usd: string
  frozen_usd: string
  spent_usd: string
}

export async function getAdminVouchers(
  filters: { status: string; purpose: string },
  page: number,
  pageSize: number
): Promise<VoucherPage> {
  const res = await api.get<ApiResponse<VoucherPage>>(`${BASE}/vouchers`, {
    params: {
      status: filters.status || undefined,
      purpose: filters.purpose || undefined,
      p: page,
      page_size: pageSize,
    },
  })
  return requireServerSuccess(res.data).data
}

export async function getVoucherIssuance(): Promise<VoucherIssuance> {
  const res = await api.get<ApiResponse<VoucherIssuance>>(
    `${BASE}/vouchers/limit`
  )
  return requireServerSuccess(res.data).data
}

export async function saveVoucherIssuanceLimit(
  limitUsd: string
): Promise<void> {
  const res = await api.put<ApiResponse<unknown>>(`${BASE}/vouchers/limit`, {
    limit_usd: limitUsd,
  })
  requireServerSuccess(res.data)
}

export async function issueMainUserVoucher(body: {
  amount_usd: string
  reason: string
}): Promise<VoucherIssued> {
  const res = await api.post<ApiResponse<VoucherIssued>>(`${BASE}/vouchers`, {
    purpose: 'main_user',
    ...body,
  })
  return requireServerSuccess(res.data).data
}

export async function freezeAdminVoucher(id: number): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/vouchers/${id}/freeze`
  )
  requireServerSuccess(res.data)
}

export async function cancelAdminVoucher(id: number): Promise<void> {
  const res = await api.post<ApiResponse<unknown>>(
    `${BASE}/vouchers/${id}/cancel`
  )
  requireServerSuccess(res.data)
}

export async function getAdminVoucherMarginAuthorization(
  siteId: number
): Promise<{ site_id: number; allowed: boolean }> {
  const res = await api.get<ApiResponse<{ site_id: number; allowed: boolean }>>(
    `${BASE}/vouchers/sites/${siteId}/margin`
  )
  return requireServerSuccess(res.data).data
}

export async function saveAdminVoucherMarginAuthorization(
  siteId: number,
  allowed: boolean
): Promise<void> {
  const res = await api.put<ApiResponse<unknown>>(
    `${BASE}/vouchers/sites/${siteId}/margin`,
    { allowed }
  )
  requireServerSuccess(res.data)
}

/** One site level (model.AdtySiteTier). Decimals are strings; empty cost factor or markup uses the default. */
export type SiteTier = {
  id?: number
  code: string
  name: string
  min_effective_users: number
  min_principal_usd: string
  min_consumption_usd: string
  cost_factor: string | null
  max_markup: string | null
  deposit_usd: string
  enabled: boolean
  sort_order: number
}

export async function getSiteTiers(): Promise<SiteTier[]> {
  const res = await api.get<ApiResponse<SiteTier[]>>(`${BASE}/reseller/tiers`)
  return listOrEmpty(requireServerSuccess(res.data).data)
}

/** Upserts one level by code; omitted levels are left unchanged, never deleted. */
export async function saveSiteTier(tier: SiteTier): Promise<SiteTier[]> {
  const res = await api.put<ApiResponse<SiteTier[]>>(`${BASE}/reseller/tiers`, {
    tiers: [tier],
  })
  return listOrEmpty(requireServerSuccess(res.data).data)
}

export async function setSiteTierAssignment(
  siteId: number,
  code: string,
  mode: 'auto' | 'manual'
): Promise<void> {
  const res = await api.put<ApiResponse<unknown>>(
    `${BASE}/sites/${siteId}/tier`,
    { code, mode }
  )
  requireServerSuccess(res.data)
}
