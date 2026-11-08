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
import { expect, it } from 'vitest'

import { cleanBackupCode, formatBackupCode } from '../validation'

it.each([
  { condition: 'empty input', input: '', expected: '' },
  { condition: 'four characters', input: 'a1b2', expected: 'A1B2' },
  { condition: 'a fifth character', input: 'a1b2c', expected: 'A1B2-C' },
  { condition: 'eight characters', input: 'a1b2c3d4', expected: 'A1B2-C3D4' },
  {
    condition: 'excess characters',
    input: 'a1b2c3d4e5',
    expected: 'A1B2-C3D4',
  },
  {
    condition: 'multiple separators',
    input: ' ab-12!cd.34\ndef ',
    expected: 'AB12-CD34',
  },
])('formats backup codes with $condition', ({ input, expected }) => {
  expect(formatBackupCode(input)).toBe(expected)
})

it('applies uppercase expansion before filtering and truncating a backup code', () => {
  expect(formatBackupCode('ßabc1234')).toBe('SSAB-C123')
})

it.each([
  { condition: 'empty input', input: '', expected: '' },
  {
    condition: 'multiple hyphens',
    input: 'ab--cd-12--34',
    expected: 'abcd1234',
  },
  {
    condition: 'mixed case and whitespace',
    input: ' aB-12_ç--34\n',
    expected: ' aB12_ç34\n',
  },
  { condition: 'a non-ASCII dash', input: 'AB–CD-12', expected: 'AB–CD12' },
])(
  'removes only ASCII hyphens from backup codes with $condition',
  ({ input, expected }) => {
    expect(cleanBackupCode(input)).toBe(expected)
  }
)
