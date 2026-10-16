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

import { formatQuota } from '@/lib/format'

import type { LedgerEntry, WalletLedgerSelfParams } from '../types'

export const DEFAULT_LEDGER_LIMIT = 50

export const LEDGER_KIND_OPTIONS = [
  'topup_paid',
  'topup_tier_bonus',
  'topup_first_bonus',
  'api_consume_hourly',
  'consume_arrears',
  'topup_refund',
  'topup_refund_bonus_claw',
  'balance_adjust',
  'redemption',
] as const

/** Cap mirrors backend: limit <= 0 or > 100 → 50. */
export function clampLedgerLimit(limit: number | undefined | null): number {
  const n = Number(limit)
  if (!Number.isFinite(n) || n <= 0 || n > 100) return DEFAULT_LEDGER_LIMIT
  return Math.floor(n)
}

export function clampLedgerOffset(offset: number | undefined | null): number {
  const n = Number(offset)
  if (!Number.isFinite(n) || n < 0) return 0
  return Math.floor(n)
}

/**
 * Build query for GET /api/wallet/ledger/self.
 * Omits empty filters; normalizes limit/offset.
 */
export function buildWalletLedgerSelfQuery(
  params: WalletLedgerSelfParams
): Record<string, string | number> {
  const out: Record<string, string | number> = {
    limit: clampLedgerLimit(params.limit),
    offset: clampLedgerOffset(params.offset),
  }
  const account = (params.account || '').trim()
  if (account === 'paid' || account === 'bonus') {
    out.account = account
  }
  const kind = (params.kind || '').trim()
  if (kind) out.kind = kind
  if (params.created_from != null && params.created_from !== '') {
    out.created_from = params.created_from
  }
  if (params.created_to != null && params.created_to !== '') {
    out.created_to = params.created_to
  }
  return out
}

/**
 * Split one ledger row into paid / bonus delta columns.
 * Only the matching account column gets delta; the other is null (UI shows —).
 */
export function splitAccountColumns(
  entry: Pick<LedgerEntry, 'account' | 'delta'>
): {
  paidDelta: number | null
  bonusDelta: number | null
} {
  const acct = (entry.account || '').toLowerCase()
  if (acct === 'paid' || acct === 'quota') {
    return { paidDelta: entry.delta, bonusDelta: null }
  }
  if (acct === 'bonus') {
    return { paidDelta: null, bonusDelta: entry.delta }
  }
  return { paidDelta: null, bonusDelta: null }
}

/** Stable English keys for t(); keep in sync with locale files. */
export function ledgerKindLabelKey(kind: string): string {
  switch (kind) {
    case 'topup_paid':
      return 'Ledger kind topup paid'
    case 'topup_tier_bonus':
      return 'Ledger kind tier bonus'
    case 'topup_first_bonus':
      return 'Ledger kind first bonus'
    case 'api_consume_hourly':
      return 'Ledger kind api consume'
    case 'consume_arrears':
      return 'Ledger kind arrears'
    case 'topup_refund':
      return 'Ledger kind refund'
    case 'topup_refund_bonus_claw':
      return 'Ledger kind bonus claw'
    case 'balance_adjust':
      return 'Ledger kind balance adjust'
    case 'redemption':
      return 'Ledger kind redemption'
    default:
      return kind || 'Unknown'
  }
}

/** RFC3339 / ISO string → local locale; empty → em dash. */
export function formatLedgerTime(
  iso: string | null | undefined,
  locale?: string
): string {
  if (!iso) return '—'
  const ms = Date.parse(iso)
  if (!Number.isFinite(ms)) return iso
  try {
    return new Date(ms).toLocaleString(locale)
  } catch {
    return new Date(ms).toLocaleString()
  }
}

/** Signed quota display (+$x / -$x / —). */
export function formatSignedQuota(delta: number | null | undefined): string {
  if (delta == null || Number.isNaN(Number(delta))) return '—'
  const n = Number(delta)
  if (n === 0) return formatQuota(0)
  const abs = formatQuota(Math.abs(n))
  if (n > 0) return `+${abs}`
  return `-${abs.replace(/^-/, '')}`
}

/** Local datetime-local value → unix seconds; empty → undefined. */
export function datetimeLocalToUnix(raw: string): number | undefined {
  const s = raw.trim()
  if (!s) return undefined
  const ms = Date.parse(s)
  if (!Number.isFinite(ms)) return undefined
  return Math.floor(ms / 1000)
}

export function parseUserIdFilter(raw: string): number | undefined {
  const s = raw.trim()
  if (!s) return undefined
  const n = Number(s)
  if (!Number.isInteger(n) || n <= 0) return undefined
  return n
}

/** Backend ledger reason literals (model.*: "adtyai …") → i18n key; unknown → null. */
const ledgerReasonLabelKeys: Readonly<Record<string, string>> = {
  'adtyai topup paid': 'Top-up payment',
  'adtyai topup bonus': 'Top-up bonus',
  'adtyai redemption bonus': 'Redemption bonus',
  'adtyai first topup extra approved': 'First top-up extra bonus (approved)',
  // model/ledger.go 每小时 API 消费汇总
  'hourly api consume rollup': 'Hourly API usage rollup',
  // model/quota_reserve.go 预扣失败退回
  'restore pre-consumed wallet': 'Pre-deducted amount returned',
  // aff 账户 reason（refund void frozen commission、refund claw available
  // commission、void aff commission、reject withdraw unfreeze、aff_commission、
  // aff_unfreeze、aff_withdraw_freeze）不映射：用户钱包流水接口只返回
  // paid+bonus（controller/wallet_ledger_adty.go），后台赠送流水固定
  // account=bonus，前端没有展示 aff 账户流水的页面。
}

export function ledgerReasonLabelKey(reason: string): string | null {
  return Object.hasOwn(ledgerReasonLabelKeys, reason)
    ? ledgerReasonLabelKeys[reason]
    : null
}

export function ledgerNote(
  entry: Pick<LedgerEntry, 'reason' | 'ref_id' | 'ref_type'>,
  translate?: (key: string) => string
): string {
  const raw = (entry.reason || '').trim()
  const key = raw ? ledgerReasonLabelKey(raw) : null
  const reason = key && translate ? translate(key) : raw
  const ref = (entry.ref_id || '').trim()
  if (reason && ref) return `${reason} · ${ref}`
  return reason || ref || '—'
}
