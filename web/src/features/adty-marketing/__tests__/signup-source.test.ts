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

import { buildSignupSource } from '../lib/signup-source'

const site = { hostname: 'adtyai.example' }

function decode(value: string): Record<string, string> {
  return Object.fromEntries(new URLSearchParams(decodeURIComponent(value)))
}

describe('buildSignupSource', () => {
  it('records UTM parameters, the landing path and an outside referrer', () => {
    const value = buildSignupSource(
      {
        ...site,
        pathname: '/pricing',
        search: '?utm_source=google&utm_medium=cpc&utm_campaign=launch&x=1',
      },
      'https://news.ycombinator.com/item?id=1',
      ''
    )
    expect(decode(value)).toEqual({
      utm_source: 'google',
      utm_medium: 'cpc',
      utm_campaign: 'launch',
      landing: '/pricing',
      ref_host: 'news.ycombinator.com',
    })
  })

  it('leaves out a referrer from the same site', () => {
    const value = buildSignupSource(
      { ...site, pathname: '/', search: '' },
      'https://adtyai.example/docs',
      ''
    )
    expect(decode(value)).toEqual({ landing: '/' })
  })

  it('keeps the first visit when the cookie already exists', () => {
    expect(
      buildSignupSource(
        { ...site, pathname: '/', search: '?utm_source=bing' },
        '',
        'adty_did=1; adty_src=abc'
      )
    ).toBe('')
  })
})
