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
import type { TFunction } from 'i18next'

/** One recharge code (model/adty_voucher.go). Only the last characters are stored. */
export type Voucher = {
  id: number
  code_tail: string
  purpose: string
  status: string
  amount_usd: string
  quota: number
  source_kind: string
  source_site_id: number
  target_site_id: number
  issuer_id: number
  redeemer_id: number
  inherited_by_main: boolean
  trade_no: string
  created_at: number
  redeemed_at: number
}

export type VoucherPage = {
  total: number
  items: Voucher[]
  page: number
  page_size: number
}

/** The response of a successful issue; `code` is shown once and never again. */
export type VoucherIssued = {
  id: number
  code: string
  code_tail: string
  purpose: string
  amount_usd: string
  quota: number
}

export const VOUCHER_STATUSES = [
  'pending',
  'frozen',
  'redeemed',
  'cancelled',
] as const

/** Literal t() calls so the key scanner sees every label. */
export function voucherStatusLabel(t: TFunction, status: string): string {
  switch (status) {
    case 'pending':
      return t('Not redeemed')
    case 'frozen':
      return t('Frozen')
    case 'redeemed':
      return t('Redeemed')
    case 'cancelled':
      return t('Cancelled')
    default:
      return status
  }
}

export function voucherPurposeLabel(t: TFunction, purpose: string): string {
  switch (purpose) {
    case 'main_user':
      return t('Main-site user principal')
    case 'site_margin':
      return t('Site owner margin')
    case 'site_customer':
      return t('Customer principal')
    case 'site_owner':
      return t('My spending principal')
    default:
      return purpose
  }
}

export function voucherSourceLabel(t: TFunction, source: string): string {
  switch (source) {
    case 'platform':
      return t('Platform issuance limit')
    case 'earnings':
      return t('Earnings wallet')
    case 'margin':
      return t('Margin wallet')
    default:
      return source
  }
}

/** Codes that can still be frozen (pending only) or cancelled (pending or frozen). */
export function voucherCanFreeze(voucher: Voucher) {
  return voucher.status === 'pending'
}

export function voucherCanCancel(voucher: Voucher) {
  return voucher.status === 'pending' || voucher.status === 'frozen'
}
