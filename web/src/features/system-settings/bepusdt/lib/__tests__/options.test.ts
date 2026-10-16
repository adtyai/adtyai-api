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
} from '../options'

describe('parseBepusdtOptions', () => {
  it('reads five keys and token secret_set/fingerprint', () => {
    const parsed = parseBepusdtOptions([
      { key: 'BepusdtBaseURL', value: ' https://pay.example.com/ ' },
      { key: 'BepusdtLegacyNotifyEnabled', value: 'true' },
      { key: 'BepusdtTradeType', value: 'usdt.erc20' },
      { key: 'BepusdtCurrency', value: 'usd' },
      { key: 'BepusdtApiToken.secret_set', value: 'true' },
      { key: 'BepusdtApiToken.fingerprint', value: 'hm_abc123' },
    ])

    expect(parsed.form.BepusdtBaseURL).toBe('https://pay.example.com/')
    expect(parsed.form.BepusdtApiToken).toBe('')
    expect(parsed.form.BepusdtLegacyNotifyEnabled).toBe(true)
    expect(parsed.form.BepusdtTradeType).toBe('usdt.erc20')
    expect(parsed.form.BepusdtCurrency).toBe('USD')
    expect(parsed.token.secretSet).toBe(true)
    expect(parsed.token.fingerprint).toBe('hm_abc123')
  })

  it('falls back to defaults when missing', () => {
    const parsed = parseBepusdtOptions([])
    expect(parsed.form).toEqual(BEPUSDT_DEFAULTS)
    expect(parsed.token.secretSet).toBe(false)
    expect(parsed.token.fingerprint).toBe('')
  })
})

describe('sanitizeBepusdtForm + buildBepusdtUpdates', () => {
  it('strips trailing slash and uppercases currency', () => {
    expect(
      sanitizeBepusdtForm({
        ...BEPUSDT_DEFAULTS,
        BepusdtBaseURL: 'https://pay.example.com///',
        BepusdtCurrency: 'usd',
        BepusdtTradeType: 'USDT.TRC20',
      })
    ).toEqual({
      BepusdtBaseURL: 'https://pay.example.com',
      BepusdtApiToken: '',
      BepusdtLegacyNotifyEnabled: false,
      BepusdtTradeType: 'usdt.trc20',
      BepusdtCurrency: 'USD',
    })
  })

  it('omits empty token and only emits changed keys', () => {
    const initial = {
      ...BEPUSDT_DEFAULTS,
      BepusdtBaseURL: 'https://a.example',
      BepusdtLegacyNotifyEnabled: false,
    }
    const next = {
      ...initial,
      BepusdtBaseURL: 'https://b.example',
      BepusdtApiToken: '   ',
      BepusdtLegacyNotifyEnabled: true,
    }
    expect(buildBepusdtUpdates(next, initial)).toEqual([
      { key: 'BepusdtBaseURL', value: 'https://b.example' },
      { key: 'BepusdtLegacyNotifyEnabled', value: 'true' },
    ])
  })

  it('includes token only when non-empty', () => {
    const initial = { ...BEPUSDT_DEFAULTS }
    const next = { ...initial, BepusdtApiToken: 'new-secret' }
    expect(buildBepusdtUpdates(next, initial)).toEqual([
      { key: 'BepusdtApiToken', value: 'new-secret' },
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
