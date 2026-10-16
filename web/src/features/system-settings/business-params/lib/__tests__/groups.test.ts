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
import { describe, expect, it } from 'vitest'

import {
  BUSINESS_PARAM_GROUPS,
  BUSINESS_PARAM_PAGE_LABELS,
  formatDisplayValue,
  formatPageLabel,
  formatRange,
  formatSnapshot,
  groupRows,
  isValidDailyReportTime,
  isValidReconcileFromDate,
  parsePutValue,
  settingKeyLabel,
} from '../groups'
import type { BusinessParamRow } from '../../types'

function row(
  partial: Partial<BusinessParamRow> & Pick<BusinessParamRow, 'key'>
): BusinessParamRow {
  return {
    type: 'number',
    default: '0',
    value: '0',
    ...partial,
  }
}

describe('business-params groups', () => {
  it('covers 31 catalog keys across groups (no EpayInstanceDailyLimit)', () => {
    const keys = BUSINESS_PARAM_GROUPS.flatMap((g) => [...g.keys])
    expect(keys).toHaveLength(31)
    expect(keys).not.toContain('EpayInstanceDailyLimit')
    expect(new Set(keys).size).toBe(31)
  })

  it('groups rows and keeps leftover in Other', () => {
    const rows = [
      row({ key: 'FirstTopupBonusRate', value: '0.10' }),
      row({ key: 'Price', value: '7.3' }),
      row({ key: 'UnknownFutureKey', value: '1' }),
    ]
    const grouped = groupRows(rows)
    expect(grouped.map((g) => g.group.titleKey)).toEqual([
      'Top-up',
      'Payment',
      'Other',
    ])
  })

  it('formats snapshot, range, and TopupBonusTiers display', () => {
    expect(formatSnapshot('order')).toContain('New orders')
    expect(formatSnapshot('')).toContain('immediately')
    expect(
      formatRange(
        row({
          key: 'FirstTopupBonusRate',
          min: 0,
          max: 1,
        })
      )
    ).toBe('0–1')
    expect(
      formatDisplayValue(
        row({
          key: 'TopupBonusTiers',
          type: 'json',
          value: '[1,2,8]',
        })
      )
    ).toBe('3 tiers · 1%–8%')
  })

  it('parsePutValue coerces number and json', () => {
    expect(
      parsePutValue(row({ key: 'Price', type: 'number' }), '7.3')
    ).toBe(7.3)
    expect(
      parsePutValue(
        row({ key: 'TopupBonusTiers', type: 'json' }),
        '[1,2,3]'
      )
    ).toEqual([1, 2, 3])
  })

  it('validates DailyReportTime as HH:mm', () => {
    expect(isValidDailyReportTime('09:00')).toBe(true)
    expect(isValidDailyReportTime('23:59')).toBe(true)
    expect(isValidDailyReportTime('9:00')).toBe(false)
    expect(isValidDailyReportTime('24:00')).toBe(false)
    expect(isValidDailyReportTime('')).toBe(false)
    expect(
      parsePutValue(
        row({ key: 'DailyReportTime', type: 'string', value: '09:00' }),
        '09:00'
      )
    ).toBe('09:00')
    expect(() =>
      parsePutValue(
        row({ key: 'DailyReportTime', type: 'string', value: '09:00' }),
        '9:00'
      )
    ).toThrow('Daily report time must be HH:mm')
    expect(formatRange(row({ key: 'DailyReportTime', type: 'string' }))).toBe(
      'HH:mm (Asia/Shanghai)'
    )
  })

  it('validates WalletAccountReconcileFrom as empty or YYYY-MM-DD', () => {
    const r = row({ key: 'WalletAccountReconcileFrom', type: 'string' })
    expect(isValidReconcileFromDate('')).toBe(true)
    expect(isValidReconcileFromDate('2026-10-16')).toBe(true)
    expect(isValidReconcileFromDate('2026-02-30')).toBe(false)
    expect(isValidReconcileFromDate('2026/10/16')).toBe(false)
    expect(parsePutValue(r, ' 2026-10-16 ')).toBe('2026-10-16')
    expect(parsePutValue(r, '')).toBe('')
    expect(() => parsePutValue(r, '20261016')).toThrow(
      'Start date must be YYYY-MM-DD or empty'
    )
    expect(formatRange(r)).toBe('YYYY-MM-DD (Asia/Shanghai); empty = off')
    expect(settingKeyLabel('WalletAccountReconcileFrom')).toBe(
      'Per-account reconcile start date'
    )
  })
})

describe('formatPageLabel', () => {
  it('maps backend page codes to console page names', () => {
    expect(formatPageLabel('A29')).toBe('Chatwoot customer support')
    expect(formatPageLabel('A43')).toBe('Telegram alerts')
    expect(formatPageLabel('A13')).toBe('BEpusdt gateway')
  })
  it('maps combined codes and dedupes', () => {
    expect(formatPageLabel('A22/A43')).toBe(
      'Not enabled in this version / Telegram alerts'
    )
    expect(formatPageLabel('A10/A11')).toBe('Billing & Payment')
  })
  it('keeps unknown codes raw and handles empty', () => {
    expect(formatPageLabel('A99')).toBe('A99')
    expect(formatPageLabel(undefined)).toBe('')
  })
  it('maps A38 (Epay instance editor) onto the Epay instances page', () => {
    expect(formatPageLabel('A38')).toBe('Epay instances')
    expect(formatPageLabel('A12/A38')).toBe('Epay instances')
  })
  it('passes labels through translate', () => {
    expect(formatPageLabel('A43', (k) => `t:${k}`)).toBe('t:Telegram alerts')
  })
})

describe('settingKeyLabel', () => {
  it('labels AdtyTelegramBotToken without exposing the raw key', () => {
    expect(settingKeyLabel('AdtyTelegramBotToken')).toBe('Telegram bot token')
  })
  it('falls back to business param descriptions then raw key', () => {
    expect(settingKeyLabel('DiskAlertWarnPct')).toBe('Disk warn line %')
    expect(settingKeyLabel('Nope')).toBe('Nope')
  })
})

describe('backend business param catalog coverage', () => {
  it('every key in model/business_params.go has a label (no raw key shown)', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const src = readFileSync(
      resolve(process.cwd(), '../model/business_params.go'),
      'utf8'
    )
    const keys = [...src.matchAll(/Key:\s*"([A-Za-z0-9]+)"/g)].map((m) => m[1])
    expect(keys.length).toBeGreaterThanOrEqual(30)
    const missing = keys.filter((k) => settingKeyLabel(k) === k)
    expect(missing).toEqual([])
  })
  it('every page code in model/business_params.go has a page name', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const src = readFileSync(
      resolve(process.cwd(), '../model/business_params.go'),
      'utf8'
    )
    const codes = [...src.matchAll(/Page:\s*"([^"]*)"/g)].flatMap((m) =>
      m[1]
        .split('/')
        .map((p) => p.trim())
        .filter(Boolean)
    )
    expect(codes.length).toBeGreaterThanOrEqual(30)
    const missing = [...new Set(codes)].filter(
      (c) => !Object.hasOwn(BUSINESS_PARAM_PAGE_LABELS, c)
    )
    expect(missing).toEqual([])
  })
  it('excluded EpayInstanceDailyLimit has a label', () => {
    expect(settingKeyLabel('EpayInstanceDailyLimit')).toBe(
      'Epay instance daily limit'
    )
  })
})

describe('parsePutValue error keys', () => {
  it('throws i18n keys, never raw SyntaxError', () => {
    expect(() =>
      parsePutValue(
        { key: 'X', type: 'json' } as Parameters<typeof parsePutValue>[0],
        '{bad'
      )
    ).toThrow('Invalid JSON format')
  })
})
