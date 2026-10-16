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
  parseGroupDisplayNames,
  resolveGroupDisplayName,
} from '../group-display-name'

describe('group display names (A08)', () => {
  const map = parseGroupDisplayNames({ vip: 'VIP 通道', blank: '  ', n: 1 })

  it('maps code to display name', () => {
    expect(resolveGroupDisplayName('vip', map)).toBe('VIP 通道')
  })

  it('falls back to code when meta missing / blank / invalid', () => {
    expect(resolveGroupDisplayName('default', map)).toBe('default')
    expect(resolveGroupDisplayName('blank', map)).toBe('blank')
    expect(resolveGroupDisplayName('n', map)).toBe('n')
    expect(
      resolveGroupDisplayName('x', parseGroupDisplayNames(undefined))
    ).toBe('x')
    expect(resolveGroupDisplayName('x', parseGroupDisplayNames('{bad'))).toBe(
      'x'
    )
    expect(resolveGroupDisplayName('', map)).toBe('')
  })

  it('accepts JSON string payload', () => {
    expect(
      resolveGroupDisplayName('vip', parseGroupDisplayNames('{"vip":"V"}'))
    ).toBe('V')
  })
})
