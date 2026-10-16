import { describe, expect, it } from 'vitest'

import {
  BUSINESS_PARAM_DESCRIPTIONS,
  formatDisplayValue,
  formatRange,
  formatSnapshot,
} from '@/features/system-settings/business-params/lib/groups'
import type { BusinessParamRow } from '@/features/system-settings/business-params/types'
import { orderStatusLabelKey } from '@/features/system-settings/orders/lib/orders'
import { riskReasonLabelKey } from '@/features/system-settings/utils/status-labels'
import {
  ledgerNote,
  ledgerReasonLabelKey,
} from '@/features/wallet/ledger/lib/ledger'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { businessParamKeyLabel } from '@/lib/server-error-message'

const zhT = (zh as { translation: Record<string, string> }).translation
const enT = (en as { translation: Record<string, string> }).translation
const hasZh = (k: string) => {
  expect(enT[k], `en missing ${k}`).toBe(k)
  expect(zhT[k], `zh missing ${k}`).toBeTruthy()
  expect(zhT[k]).not.toBe(k)
}

describe('adty display labels', () => {
  it('risk reasons cover every backend value (DetectAffSelfInvite + topup_tiers)', () => {
    for (const r of [
      'same_email',
      'same_device_fp',
      'same_ip',
      'missing_device_fp',
      'ip_conflict',
    ]) {
      hasZh(riskReasonLabelKey(r) as string)
    }
    expect(riskReasonLabelKey('conc')).toBeNull()
  })
  it('order need_manual and unknown raw', () => {
    hasZh(orderStatusLabelKey('need_manual'))
    expect(orderStatusLabelKey('weird')).toBe('weird')
  })
  it('ledger reasons map; unknown kept', () => {
    for (const r of [
      'hourly api consume rollup',
      'restore pre-consumed wallet',
    ]) {
      hasZh(ledgerReasonLabelKey(r) as string)
    }
    for (const r of [
      'adtyai topup paid',
      'adtyai topup bonus',
      'adtyai redemption bonus',
      'adtyai first topup extra approved',
    ]) {
      hasZh(ledgerReasonLabelKey(r) as string)
    }
    const t = (k: string) => zhT[k] ?? k
    expect(
      ledgerNote({ reason: 'adtyai topup bonus', ref_id: 'ADTY1' }, t)
    ).toBe('充值赠送 · ADTY1')
    expect(ledgerNote({ reason: 'manual fix', ref_id: 'x' }, t)).toBe(
      'manual fix · x'
    )
    expect(ledgerNote({ reason: 'adtyai topup bonus', ref_id: '' })).toBe(
      'adtyai topup bonus'
    )
  })
  it('range column never passes backend notes through', () => {
    const row = {
      key: 'TopupBonusTiers',
      notes: '13 rates % matching X',
    } as BusinessParamRow
    expect(formatRange(row)).toBe('tier amounts >0 ascending; rates 0–100%')
    expect(
      formatRange({ key: 'Foo', notes: 'dev note' } as BusinessParamRow)
    ).toBe('—')
    expect(zhT['HH:mm (Asia/Shanghai)']).toBe('时:分（北京时间）')
  })
  it('business param descriptions, ranges, snapshots and tiers all translated', () => {
    for (const d of Object.values(BUSINESS_PARAM_DESCRIPTIONS)) {
      hasZh(d)
    }
    for (const s of ['order', 'affiliate', 'withdraw', 'apply', undefined]) {
      hasZh(formatSnapshot(s))
    }
    for (const key of [
      'DiskAlertCritPct',
      'DailyReportTime',
      'WalletAccountReconcileFrom',
      'BackupContact',
      'TopupBonusTiers',
    ]) {
      hasZh(formatRange({ key } as BusinessParamRow))
    }
    hasZh('{{count}} tiers · {{min}}%–{{max}}%')
    const row = {
      key: 'TopupBonusTiers',
      value: '[1,2,8]',
    } as BusinessParamRow
    expect(formatDisplayValue(row)).toBe('3 tiers · 1%–8%')
    hasZh('≈ {{usd}} USD → {{quota}} quota')
  })
  it('unknown_key / out_of_range map {key} to the param name; others untouched', () => {
    expect(
      businessParamKeyLabel('out_of_range', {
        key: 'TopupOrderTimeoutMinutes',
      })
    ).toHaveProperty('key')
    expect(businessParamKeyLabel('unknown_key', { key: 'NoSuchKey' })).toEqual(
      {}
    )
    expect(
      businessParamKeyLabel('invalid_value', {
        key: 'TopupOrderTimeoutMinutes',
      })
    ).toEqual({})
  })
  it('zh help texts no longer mention CEO confirmation', () => {
    const n = Object.values(zhT).filter((v) =>
      v.includes('\u603b\u88c1\u6700\u7ec8\u786e\u8ba4')
    ).length
    expect(n).toBe(0)
  })
})

describe('err-i18n-3', () => {
  it('backup contact types map; unknown → null', async () => {
    const { backupContactTypeLabelKey } =
      await import('@/features/system-settings/utils/status-labels')
    expect(zhT[backupContactTypeLabelKey('email') as string]).toBe('邮箱')
    expect(backupContactTypeLabelKey('telegram')).toBe('Telegram')
    expect(backupContactTypeLabelKey('sms')).toBeNull()
  })
  it('no internal page codes (A/B/U + digits) left in zh/en values', () => {
    const re = /(?<![A-Za-z0-9_])[ABU]\d{1,3}(?![0-9A-Za-z])/
    for (const v of [...Object.values(zhT), ...Object.values(enT)]) {
      expect(re.test(v), v).toBe(false)
    }
  })
  it('reconcile skipped badge has no raw ok', () => {
    expect(zhT['Skipped (already balanced that day)']).toBe(
      '已跳过（当日已平账）'
    )
  })
})
