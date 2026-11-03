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
  ReferralCommission,
  ReferralPage,
  ReferralProfile,
  ReferralPublicSettings,
  ReferralWithdrawal,
  WithdrawalRequest,
} from './types'

type ApiResponse<T> = {
  success: boolean
  message?: string
  code?: string
  data: T
}

export const REFERRAL_PAGE_SIZE = 10

export const referralQueryKeys = {
  all: ['referral'] as const,
  profile: ['referral', 'profile'] as const,
  commissions: (page: number) => ['referral', 'commissions', page] as const,
  withdrawals: (page: number) => ['referral', 'withdrawals', page] as const,
  publicSettings: ['referral', 'public-settings'] as const,
}

export async function getReferralProfile(): Promise<ReferralProfile> {
  const res = await api.get<ApiResponse<ReferralProfile>>(
    '/api/user/referral/profile'
  )
  return requireServerSuccess(res.data).data
}

export async function getReferralCommissions(
  page: number
): Promise<ReferralPage<ReferralCommission>> {
  const res = await api.get<ApiResponse<ReferralPage<ReferralCommission>>>(
    '/api/user/referral/commissions',
    { params: { p: page, page_size: REFERRAL_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

export async function getReferralWithdrawals(
  page: number
): Promise<ReferralPage<ReferralWithdrawal>> {
  const res = await api.get<ApiResponse<ReferralPage<ReferralWithdrawal>>>(
    '/api/user/referral/withdrawals',
    { params: { p: page, page_size: REFERRAL_PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

export async function applyReferralPartner(note: string): Promise<void> {
  const res = await api.post<ApiResponse<null>>('/api/user/referral/apply', {
    note,
  })
  requireServerSuccess(res.data)
}

export type RateChange = { rate: string; cnyAmount: string; updatedAt: number }

export type WithdrawalResult =
  | { kind: 'ok'; withdrawal: ReferralWithdrawal }
  | ({ kind: 'rate_changed' } & RateChange)

type RateChangedBody = { rate: string; cny_amount: string; updated_at: number }

/** Submits a withdrawal; a changed CNY rate comes back as rate_changed instead of an error. */
export async function createReferralWithdrawal(
  request: WithdrawalRequest
): Promise<WithdrawalResult> {
  const res = await api.post<ApiResponse<ReferralWithdrawal | RateChangedBody>>(
    '/api/user/referral/withdrawals',
    request
  )
  if (res.data.success === false && res.data.code === 'rate_changed') {
    const change = res.data.data as RateChangedBody
    return {
      kind: 'rate_changed',
      rate: change.rate,
      cnyAmount: change.cny_amount,
      updatedAt: change.updated_at,
    }
  }
  return {
    kind: 'ok',
    withdrawal: requireServerSuccess(res.data).data as ReferralWithdrawal,
  }
}

export async function cancelReferralWithdrawal(id: number): Promise<void> {
  const res = await api.post<ApiResponse<null>>(
    `/api/user/referral/withdrawals/${id}/cancel`
  )
  requireServerSuccess(res.data)
}

/** Uploads a QR code (user) or payout proof (admin path); returns a signed link. */
export async function uploadReferralAsset(
  file: File,
  path = '/api/user/referral/upload'
): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post<ApiResponse<{ url: string }>>(path, form)
  return requireServerSuccess(res.data).data.url
}

export async function getReferralPublicSettings(): Promise<ReferralPublicSettings> {
  const res = await api.get<ApiResponse<ReferralPublicSettings>>(
    '/api/adty/referral/public-settings'
  )
  return requireServerSuccess(res.data).data
}

/** Records a click and sets the signed referral cookie. */
export async function recordReferralClick(
  code: string,
  landingPath: string
): Promise<void> {
  const res = await api.post<ApiResponse<{ code: string }>>(
    '/api/adty/referral/click',
    { code, landing_path: landingPath }
  )
  requireServerSuccess(res.data)
}
