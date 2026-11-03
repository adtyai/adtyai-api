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

import { isValidUsdtAddress, usdtNetAmount } from '../lib/address'

const TRON = 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t'
const EVM = '0x52908400098527886E0F7030069857D2E4169EE7'

describe('isValidUsdtAddress', () => {
  it.each([
    ['TRC20', TRON, true],
    ['TRC20', EVM, false],
    ['TRC20', 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj60', false],
    ['BEP20', EVM, true],
    ['BEP20', TRON, false],
    ['POLYGON', EVM, true],
    ['POLYGON', '0x123', false],
  ] as const)('%s %s -> %s', (network, address, expected) => {
    expect(isValidUsdtAddress(network, address)).toBe(expected)
  })
})

describe('usdtNetAmount', () => {
  it('subtracts the fee and never goes below zero', () => {
    expect(usdtNetAmount(150, 2)).toBe(148)
    expect(usdtNetAmount(1.5, 2)).toBe(0)
  })
})
