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

/** Site wallet kinds (model/adty_site_finance.go). Owners never see operating. */
export type SiteWalletKind = 'operating' | 'earnings' | 'margin'

/** One row of adty_site_wallet_ledgers; amounts are USD decimal strings. */
export type SiteWalletLedgerEntry = {
  id: number
  site_id: number
  wallet_id: number
  wallet_kind: string
  delta: string
  hold_delta: string
  balance_before: string
  balance_after: string
  type: string
  ref_type: string
  ref_id: string
  reason: string
  operator_id: number
  allow_overdraft: boolean
  created_at: number
}

export type SiteWalletLedgerPage = {
  total: number
  items: SiteWalletLedgerEntry[]
  page: number
  page_size: number
}

/** A site wallet balance row (GET .../wallets). */
export type SiteWallet = {
  id: number
  kind: string
  currency?: string
  balance: string
  frozen_usd?: string
  cycle_tier_usd?: string
  cycle_used_usd?: string
}

/** Literal t() calls so the key scanner sees every label. */
export function siteWalletKindLabel(t: TFunction, kind: string): string {
  switch (kind) {
    case 'operating':
      return t('Operating wallet')
    case 'earnings':
      return t('Earnings wallet')
    case 'margin':
      return t('Margin wallet')
    default:
      return kind
  }
}

/** Ledger types written by the services that move site wallets. */
export function siteWalletLedgerTypeLabel(t: TFunction, type: string): string {
  switch (type) {
    case 'principal_credit':
      return t('Customer top-up principal')
    case 'principal_consume':
      return t('Customer principal used')
    case 'site_earning_settlement':
      return t('Earnings settlement')
    case 'earning_withdraw_freeze':
      return t('Earnings withdrawal')
    case 'earning_withdraw_release':
      return t('Withdrawal returned')
    case 'earning_to_margin':
      return t('Earnings moved to margin')
    case 'margin_cycle_open':
      return t('Margin cycle opened')
    case 'commission_freeze':
      return t('Commission held')
    case 'commission_release':
      return t('Commission hold released')
    case 'gift_reserve':
      return t('Bonus reserved')
    case 'gift_release':
      return t('Bonus reserve released')
    case 'gift_spend':
      return t('Bonus paid out')
    case 'voucher_freeze':
      return t('Recharge code issued')
    case 'voucher_margin':
      return t('Margin code redeemed')
    case 'voucher_release':
      return t('Recharge code cancelled')
    case 'voucher_spend':
      return t('Recharge code redeemed')
    default:
      return type
  }
}

/** Site earnings withdrawal statuses (model/adty_site_withdrawal.go). */
export function siteWithdrawalStatusLabel(
  t: TFunction,
  status: string
): string {
  switch (status) {
    case 'pending':
      return t('Pending review')
    case 'paying':
      return t('Paying')
    case 'paid':
      return t('Paid out')
    case 'rejected':
      return t('Rejected')
    case 'cancelled':
      return t('Cancelled')
    default:
      return status
  }
}
