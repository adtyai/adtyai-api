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
  BEPUSDT_DEFAULTS,
  buildBepusdtUpdates,
  formatTokenFingerprint,
  parseBepusdtOptions,
  sanitizeBepusdtForm,
  type BepusdtFormValues,
} from '../options'

describe('parseBepusdtOptions', () => {
  it('reads gateway fields and enabled chains', () => {
    const parsed = parseBepusdtOptions([
      { key: 'BepusdtBaseURL', value: ' https://pay.example.com/ ' },
      { key: 'BepusdtLegacyNotifyEnabled', value: 'true' },
      { key: 'BepusdtEnabledChains', value: '["xlayer","trc20","erc20"]' },
      { key: 'BepusdtApiToken.secret_set', value: 'true' },
      { key: 'BepusdtApiToken.fingerprint', value: 'hm_abc123' },
    ])

    expect(parsed.form.BepusdtBaseURL).toBe('https://pay.example.com/')
    expect(parsed.form.BepusdtApiToken).toBe('')
    expect(parsed.form.BepusdtLegacyNotifyEnabled).toBe(true)
    expect(parsed.form.enabledChains).toEqual(['trc20', 'xlayer'])
    expect(parsed.token.secretSet).toBe(true)
    expect(parsed.token.fingerprint).toBe('hm_abc123')
  })

  it('falls back to all four chains when the option is missing', () => {
    const parsed = parseBepusdtOptions([])
    expect(parsed.form).toEqual(BEPUSDT_DEFAULTS)
    expect(parsed.form.enabledChains).toEqual([
      'trc20',
      'bep20',
      'polygon',
      'xlayer',
    ])
  })

  it('fails closed when stored chains are not JSON', () => {
    const parsed = parseBepusdtOptions([
      { key: 'BepusdtEnabledChains', value: 'usdt.trc20' },
    ])
    expect(parsed.form.enabledChains).toEqual([])
  })
})

describe('sanitizeBepusdtForm + buildBepusdtUpdates', () => {
  it('strips trailing slash and keeps catalog chain order', () => {
    expect(
      sanitizeBepusdtForm({
        ...BEPUSDT_DEFAULTS,
        BepusdtBaseURL: 'https://pay.example.com///',
        enabledChains: ['xlayer', 'trc20'],
      })
    ).toEqual({
      BepusdtBaseURL: 'https://pay.example.com',
      BepusdtApiToken: '',
      BepusdtLegacyNotifyEnabled: false,
      enabledChains: ['trc20', 'xlayer'],
    })
  })

  it('omits empty token and locks currency to USD when chains change', () => {
    const initial: BepusdtFormValues = {
      ...BEPUSDT_DEFAULTS,
      BepusdtBaseURL: 'https://a.example',
      enabledChains: ['trc20', 'bep20', 'polygon', 'xlayer'],
    }
    const next: BepusdtFormValues = {
      ...initial,
      BepusdtBaseURL: 'https://b.example',
      BepusdtApiToken: '   ',
      enabledChains: ['trc20'],
    }
    expect(buildBepusdtUpdates(next, initial)).toEqual([
      { key: 'BepusdtBaseURL', value: 'https://b.example' },
      { key: 'BepusdtEnabledChains', value: '["trc20"]' },
      { key: 'BepusdtCurrency', value: 'USD' },
    ])
  })

  it('includes token only when non-empty and still writes USD', () => {
    const initial = { ...BEPUSDT_DEFAULTS }
    const next = { ...initial, BepusdtApiToken: 'new-secret' }
    expect(buildBepusdtUpdates({ ...next }, { ...initial })).toEqual([
      { key: 'BepusdtApiToken', value: 'new-secret' },
      { key: 'BepusdtCurrency', value: 'USD' },
    ])
  })
})

describe('formatTokenFingerprint', () => {
  it('shows dash when missing', () => {
    expect(formatTokenFingerprint('')).toBe('-')
    expect(formatTokenFingerprint(null)).toBe('-')
    expect(formatTokenFingerprint('hm_x')).toBe('hm_x')
  })
})
