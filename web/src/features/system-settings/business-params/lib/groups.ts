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
import type { BusinessParamGroupId, BusinessParamRow } from '../types'

export type BusinessParamGroup = {
  id: BusinessParamGroupId
  titleKey: string
  keys: readonly string[]
}

/** Groups align with A45 mock: 充值 / 支付 / 返佣 / 调额 / 告警运维 / 客服 */
export const BUSINESS_PARAM_GROUPS: readonly BusinessParamGroup[] = [
  {
    id: 'topup',
    titleKey: 'Top-up',
    keys: [
      'TopupBonusTiers',
      'FirstTopupBonusRate',
      'QuotaForNewUser',
      'BalanceLowAlertThreshold',
    ],
  },
  {
    id: 'payment',
    titleKey: 'Payment',
    keys: [
      'Price',
      'BepusdtOrderTimeoutMinutes',
      'UsdtUnderpayTolerance',
      'TopupOrderTimeoutMinutes',
      'EpayFailureDemoteThreshold',
    ],
  },
  {
    id: 'affiliate',
    titleKey: 'Affiliate',
    keys: [
      'AffTopupCommissionRate',
      'AffSubscriptionCommissionRate',
      'AffFreezeDays',
      'AffMinWithdrawAmount',
      'AffWithdrawFeeRate',
    ],
  },
  {
    id: 'adjust',
    titleKey: 'Balance adjust',
    keys: ['BalanceAdjustMaxAmount', 'SelfApproveAlertAmount'],
  },
  {
    id: 'alerts',
    titleKey: 'Alerts & ops',
    keys: [
      'ChannelGracePeriodSeconds',
      'PayUnknownEscalateHours',
      'TopupOrderRateLimitNum',
      'TopupOrderRateLimitDuration',
      'DiskAlertWarnPct',
      'DiskAlertCritPct',
      'ChannelBalanceAlertUsd',
      'DailyReportTime',
      'WalletAccountReconcileFrom',
      'BackupRetentionDays',
      'AlertMergeMinutes',
      'RefundAlertAmount',
      'RefundAlertDailyCount',
    ],
  },
  {
    id: 'support',
    titleKey: 'Support',
    keys: ['BackupContactType', 'BackupContact'],
  },
] as const

/** Chinese/English descriptions matching A45 说明 column */
export const BUSINESS_PARAM_DESCRIPTIONS: Record<string, string> = {
  TopupBonusTiers: 'Top-up tiers and bonus ladder',
  FirstTopupBonusRate: 'First top-up bonus rate (max with tier)',
  QuotaForNewUser: 'New-user gift quota (into bonus wallet)',
  BalanceLowAlertThreshold: 'User balance low-alert threshold',
  Price: 'Top-up settle FX CNY/USD (official)',
  BepusdtOrderTimeoutMinutes: 'USDT order timeout (minutes)',
  UsdtUnderpayTolerance: 'USDT underpay tolerance',
  TopupOrderTimeoutMinutes: 'Top-up order timeout (minutes)',
  EpayFailureDemoteThreshold: 'Epay consecutive-failure demote threshold',
  AffTopupCommissionRate: 'Top-up affiliate commission (on paid amount)',
  AffSubscriptionCommissionRate: 'Subscription affiliate commission',
  AffFreezeDays: 'Affiliate freeze days',
  AffMinWithdrawAmount: 'Minimum withdraw amount',
  AffWithdrawFeeRate: 'Withdraw fee rate',
  BalanceAdjustMaxAmount: 'Balance-adjust single-request max',
  SelfApproveAlertAmount: 'Solo-admin self-approve escalate threshold',
  ChannelGracePeriodSeconds: 'Channel grace period after save (seconds)',
  PayUnknownEscalateHours: 'pay_unknown escalate hours',
  TopupOrderRateLimitNum: 'Top-up order rate limit count (per user)',
  TopupOrderRateLimitDuration: 'Top-up order rate limit window (seconds)',
  DiskAlertWarnPct: 'Disk warn line %',
  DiskAlertCritPct: 'Disk critical line %',
  ChannelBalanceAlertUsd: 'Upstream channel balance alert (USD)',
  DailyReportTime: 'Daily report push time (HH:mm, Asia/Shanghai)',
  WalletAccountReconcileFrom: 'Per-account reconcile start date',
  BackupRetentionDays: 'Backup retention days',
  AlertMergeMinutes: 'Same-alert merge window (minutes)',
  RefundAlertAmount: 'Single refund escalate threshold',
  RefundAlertDailyCount: 'Same-admin daily refund count escalate',
  BackupContactType: 'Backup contact type when Chatwoot is down',
  BackupContact: 'Backup contact (email or @handle)',
}

/** Non-business-param setting keys that may surface in UI / error params. */
export const SETTING_KEY_LABELS: Record<string, string> = {
  AdtyTelegramBotToken: 'Telegram bot token',
  EpayInstanceDailyLimit: 'Epay instance daily limit',
}

/** English source label for a setting key; unknown keys return the raw key. */
export function settingKeyLabel(key: string): string {
  if (Object.hasOwn(SETTING_KEY_LABELS, key)) return SETTING_KEY_LABELS[key]
  if (Object.hasOwn(BUSINESS_PARAM_DESCRIPTIONS, key)) {
    return BUSINESS_PARAM_DESCRIPTIONS[key]
  }
  return key
}

/** Backend `page` codes (A29, A43 …) → console page title (English source). */
export const BUSINESS_PARAM_PAGE_LABELS: Record<string, string> = {
  A04: 'Channels',
  A10: 'Billing & Payment',
  A11: 'Billing & Payment',
  A12: 'Epay instances',
  A13: 'BEpusdt gateway',
  A22: 'Not enabled in this version',
  A23: 'Affiliate commissions',
  A29: 'Chatwoot customer support',
  A32: 'Operations',
  A33: 'Billing & Payment',
  A36: 'Security & Limits',
  // A38 = single Epay instance editor; shipped as the edit dialog on the
  // Epay instances page (/system-settings/epay-instances).
  A38: 'Epay instances',
  A43: 'Telegram alerts',
}

/**
 * Format a backend page code (may be combined like "A22/A43") as localized
 * page names; unknown parts keep their raw value.
 */
export function formatPageLabel(
  page: string | undefined,
  translate: (key: string) => string = (k) => k
): string {
  if (!page) return ''
  const parts = page
    .split('/')
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) =>
      Object.hasOwn(BUSINESS_PARAM_PAGE_LABELS, p)
        ? translate(BUSINESS_PARAM_PAGE_LABELS[p])
        : p
    )
  return [...new Set(parts)].join(' / ')
}

export function formatRange(row: BusinessParamRow): string {
  if (row.type === 'enum' && row.enum?.length) {
    return row.enum.join(' / ')
  }
  if (row.key === 'DiskAlertCritPct') {
    return '> DiskAlertWarnPct and ≤99'
  }
  if (row.key === 'DailyReportTime') {
    return 'HH:mm (Asia/Shanghai)'
  }
  if (row.key === 'WalletAccountReconcileFrom') {
    return 'YYYY-MM-DD (Asia/Shanghai); empty = off'
  }
  if (row.key === 'BackupContact') {
    return 'valid email or @handle'
  }
  if (row.key === 'TopupBonusTiers') {
    // 后端 notes 是英文开发说明，不透传
    return 'tier amounts >0 ascending; rates 0–100%'
  }
  const hasMin = typeof row.min === 'number'
  const hasMax = typeof row.max === 'number'
  if (hasMin && hasMax) {
    return `${row.min}–${row.max}`
  }
  if (hasMin && !hasMax) {
    if (row.min === 0) return '≥0'
    if ((row.min ?? 0) > 0 && (row.min ?? 0) < 0.01) return '>0'
    return `≥${row.min}`
  }
  if (!hasMin && hasMax) {
    return `≤${row.max}`
  }
  // 后端 notes 是英文开发说明，不透传
  return '—'
}

export function formatSnapshot(snapshot?: string): string {
  switch (snapshot) {
    case 'order':
      return 'New orders only (snapshot at create)'
    case 'affiliate':
      return 'New commissions only (snapshot at create)'
    case 'withdraw':
      return 'New withdraw requests only'
    case 'apply':
      return 'New adjust requests only'
    default:
      return 'Takes effect immediately after save'
  }
}

export function formatDisplayValue(
  row: BusinessParamRow,
  translate: (key: string, opts?: Record<string, unknown>) => string = (k, o) =>
    k.replaceAll(/\{\{(\w+)\}\}/g, (_, n) => String(o?.[n] ?? ''))
): string {
  const v = row.value
  if (row.key === 'TopupBonusTiers') {
    try {
      const parsed = JSON.parse(v) as unknown
      if (Array.isArray(parsed)) {
        const nums = parsed.map(Number).filter((n) => Number.isFinite(n))
        if (nums.length) {
          const min = Math.min(...nums)
          const max = Math.max(...nums)
          return translate('{{count}} tiers · {{min}}%–{{max}}%', {
            count: nums.length,
            min,
            max,
          })
        }
      }
    } catch {
      /* fall through */
    }
  }
  if (row.key === 'UsdtUnderpayTolerance') {
    return `${v} USDT`
  }
  if (
    row.key === 'BalanceAdjustMaxAmount' ||
    row.key === 'SelfApproveAlertAmount' ||
    row.key === 'ChannelBalanceAlertUsd' ||
    row.key === 'RefundAlertAmount'
  ) {
    const n = Number(v)
    if (Number.isFinite(n)) return `$${n}`
  }
  return v || '—'
}

/** Matches backend ValidateBusinessParam DailyReportTime: HH:MM 24h. */
const DAILY_REPORT_TIME_RE = /^([01]\d|2[0-3]):([0-5]\d)$/

export function isValidDailyReportTime(value: string): boolean {
  return DAILY_REPORT_TIME_RE.test(value.trim())
}

/** Matches backend ValidateBusinessParam WalletAccountReconcileFrom: empty or YYYY-MM-DD. */
export function isValidReconcileFromDate(value: string): boolean {
  const v = value.trim()
  if (v === '') return true
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v)
  if (!m) return false
  const d = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])))
  return (
    d.getUTCFullYear() === Number(m[1]) &&
    d.getUTCMonth() === Number(m[2]) - 1 &&
    d.getUTCDate() === Number(m[3])
  )
}

export function parsePutValue(
  row: BusinessParamRow,
  raw: string
): string | number | unknown {
  const trimmed = raw.trim()
  if (row.key === 'DailyReportTime') {
    if (!isValidDailyReportTime(trimmed)) {
      throw new Error('Daily report time must be HH:mm')
    }
    return trimmed
  }
  if (row.key === 'WalletAccountReconcileFrom') {
    if (!isValidReconcileFromDate(trimmed)) {
      throw new Error('Start date must be YYYY-MM-DD or empty')
    }
    return trimmed
  }
  if (row.type === 'json') {
    try {
      return JSON.parse(trimmed) as unknown
    } catch {
      throw new Error('Invalid JSON format')
    }
  }
  if (row.type === 'number') {
    const n = Number(trimmed)
    if (!Number.isFinite(n)) {
      throw new Error('Invalid number')
    }
    return n
  }
  return trimmed
}

export function groupRows(
  rows: BusinessParamRow[]
): Array<{ group: BusinessParamGroup; rows: BusinessParamRow[] }> {
  const byKey = new Map(rows.map((r) => [r.key, r]))
  const result: Array<{ group: BusinessParamGroup; rows: BusinessParamRow[] }> =
    []
  const seen = new Set<string>()
  for (const group of BUSINESS_PARAM_GROUPS) {
    const groupRows = group.keys
      .map((k) => byKey.get(k))
      .filter((r): r is BusinessParamRow => Boolean(r))
    groupRows.forEach((r) => seen.add(r.key))
    if (groupRows.length) {
      result.push({ group, rows: groupRows })
    }
  }
  const leftover = rows.filter((r) => !seen.has(r.key))
  if (leftover.length) {
    result.push({
      group: {
        id: 'alerts',
        titleKey: 'Other',
        keys: leftover.map((r) => r.key),
      },
      rows: leftover,
    })
  }
  return result
}
