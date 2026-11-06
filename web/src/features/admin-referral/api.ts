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
  ReferralAffiliate,
  ReferralCommission,
  ReferralPage,
  ReferralWithdrawal,
} from '@/features/referral/types'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

const BASE = '/api/user/admin/referral'
export const ADMIN_REFERRAL_PAGE_SIZE = 20

export const adminReferralKeys = {
  all: ['admin-referral'] as const,
  badges: ['admin-referral', 'badges'] as const,
  overview: ['admin-referral', 'overview'] as const,
  settings: ['admin-referral', 'settings'] as const,
  list: (name: string, params: Record<string, unknown>) =>
    ['admin-referral', name, params] as const,
}

export type ReferralBadges = {
  partners: number
  pending_withdrawals: number
  approved_withdrawals: number
  total: number
}

export type ReferralOverview = {
  affiliates: number
  bindings: number
  pending: string
  available: string
  frozen: string
  withdrawn: string
}

/** Money settings are null when unset (adty convention I). */
export type ReferralSettings = {
  enabled: boolean
  default_rate: number | null
  cookie_ttl_days: number
  freeze_days: number
  min_withdraw_usd: number | null
  usdt_withdraw_fee: number | null
  cancel_window_minutes: number
  withdraw_cny_rate: string
  withdraw_cny_rate_updated_at: number
  self_invite_window_days: number
  batch_register_threshold: number
  batch_register_window_hours: number
}

export type ReferralSettingsUpdate = Partial<
  Omit<
    ReferralSettings,
    'default_rate' | 'min_withdraw_usd' | 'usdt_withdraw_fee'
  >
> & {
  default_rate?: number
  min_withdraw_usd?: number
  usdt_withdraw_fee?: number
  clear?: Array<'default_rate' | 'min_withdraw_usd' | 'usdt_withdraw_fee'>
}

export type AffiliateAction =
  | 'rate'
  | 'disable'
  | 'restore'
  | 'freeze_settlement'
  | 'restore_settlement'
  | 'freeze_withdrawal'
  | 'restore_withdrawal'
  | 'restore_referral'
  | 'adjust'

async function getPage<T>(
  path: string,
  params: Record<string, unknown>
): Promise<ReferralPage<T>> {
  const res = await api.get<ApiResponse<ReferralPage<T>>>(BASE + path, {
    params: { page_size: ADMIN_REFERRAL_PAGE_SIZE, ...params },
  })
  return requireServerSuccess(res.data).data
}

async function post(path: string, body: unknown): Promise<void> {
  const res = await api.post<ApiResponse<null>>(BASE + path, body)
  requireServerSuccess(res.data)
}

export async function getReferralBadges(): Promise<ReferralBadges> {
  const res = await api.get<ApiResponse<ReferralBadges>>(`${BASE}/badges`)
  return requireServerSuccess(res.data).data
}

export async function getReferralOverview(): Promise<ReferralOverview> {
  const res = await api.get<ApiResponse<ReferralOverview>>(`${BASE}/overview`)
  return requireServerSuccess(res.data).data
}

export const getAffiliates = (params: {
  p: number
  keyword?: string
  partner_status?: string
}) => getPage<ReferralAffiliate>('/affiliates', params)

export const getAdminWithdrawals = (params: { p: number; status?: string }) =>
  getPage<ReferralWithdrawal>('/withdrawals', params)

export const getAdminCommissions = (params: { p: number; status?: string }) =>
  getPage<ReferralCommission>('/commissions', params)

export const reviewPartner = (
  userId: number,
  body: { approve: boolean; rate?: string; reason: string }
) => post(`/affiliates/${userId}/partner`, body)

export const updateAffiliate = (
  userId: number,
  body: {
    action: AffiliateAction
    rate?: number | null
    amount?: string
    reason?: string
    idempotency_key?: string
  }
) => post(`/affiliates/${userId}/update`, body)

export const reviewWithdrawal = (
  id: number,
  body: { approve: boolean; reason: string }
) => post(`/withdrawals/${id}/review`, body)

export const payWithdrawal = (
  id: number,
  body: { txn_no: string; proof_url: string; note: string }
) => post(`/withdrawals/${id}/pay`, body)

export const runSettlement = () => post('/settlements/run', {})

export async function getReferralSettings(): Promise<ReferralSettings> {
  const res = await api.get<ApiResponse<ReferralSettings>>(`${BASE}/settings`)
  return requireServerSuccess(res.data).data
}

export async function saveReferralSettings(
  body: ReferralSettingsUpdate
): Promise<ReferralSettings> {
  const res = await api.put<ApiResponse<ReferralSettings>>(
    `${BASE}/settings`,
    body
  )
  return requireServerSuccess(res.data).data
}
