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
  discountToPercent,
  formatDiscount,
  isValidDiscount,
  percentToDiscount,
  scopeLabel,
} from '../discount'

describe('enterprise pricing discount helpers', () => {
  it('converts between percent and API discount', () => {
    expect(percentToDiscount(85)).toBe(0.85)
    expect(discountToPercent(0.85)).toBe(85)
    expect(percentToDiscount(10)).toBe(0.1)
    expect(percentToDiscount(100)).toBe(1)
  })

  it('validates API range 0.1–1', () => {
    expect(isValidDiscount(0.1)).toBe(true)
    expect(isValidDiscount(1)).toBe(true)
    expect(isValidDiscount(0.09)).toBe(false)
    expect(isValidDiscount(1.01)).toBe(false)
  })

  it('formats display and wildcard scope', () => {
    expect(formatDiscount(0.85)).toContain('85%')
    expect(formatDiscount(0.85)).toContain('0.85')
    expect(scopeLabel(null, '*')).toBe('*')
    expect(scopeLabel('  ', '*')).toBe('*')
    expect(scopeLabel('vip', '*')).toBe('vip')
  })
})
