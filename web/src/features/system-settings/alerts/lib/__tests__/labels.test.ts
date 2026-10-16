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
import { describe, expect, test } from 'vitest'

import { riskReasonLabelKey } from '@/features/system-settings/utils/status-labels'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import {
  alertKindLabelKey,
  alertKindLabelKeys,
  alertThresholdLabelKey,
  alertThresholdLabelKeys,
} from '../labels'

const zhT = (zh as { translation: Record<string, string> }).translation
const enT = (en as { translation: Record<string, string> }).translation

// Allowed Latin tokens in zh: product / protocol / currency names.
const ZH_ALLOWED = /\b(?:IP|USDT|USD|Chatwoot)\b/g

describe('alert labels', () => {
  const keys = [
    ...Object.values(alertKindLabelKeys),
    ...Object.values(alertThresholdLabelKeys),
    ...[
      'same_device_fp',
      'same_ip',
      'same_email',
      'missing_device_fp',
      'ip_conflict',
    ].map((r) => riskReasonLabelKey(r) as string),
  ]

  test('every label has en and zh text; zh has no stray English', () => {
    for (const key of keys) {
      expect(key).toBeTruthy()
      expect(enT[key], key).toBeTruthy()
      expect(zhT[key], key).toBeTruthy()
      expect(zhT[key].replaceAll(ZH_ALLOWED, ''), key).not.toMatch(
        /[A-Za-z]{2,}/
      )
    }
  })

  test('covers the backend kinds and threshold keys', () => {
    expect(Object.keys(alertKindLabelKeys)).toHaveLength(25)
    expect(Object.keys(alertThresholdLabelKeys)).toHaveLength(11)
    expect(zhT[riskReasonLabelKey('same_device_fp') as string]).toBe(
      '同设备指纹'
    )
  })

  test('unknown values return null so the raw value is shown', () => {
    expect(alertKindLabelKey('brand_new_kind')).toBeNull()
    expect(alertKindLabelKey('toString')).toBeNull()
    expect(alertThresholdLabelKey('SomeNewKey')).toBeNull()
    expect(riskReasonLabelKey('free text reason')).toBeNull()
  })
})
