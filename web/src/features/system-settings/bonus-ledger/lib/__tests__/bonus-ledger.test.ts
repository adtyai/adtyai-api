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

import { buildAdminBonusLedgerQuery, parseUserIdFilter } from '../bonus-ledger'

describe('buildAdminBonusLedgerQuery', () => {
  it('always sends limit and offset; never account', () => {
    const q = buildAdminBonusLedgerQuery({})
    expect(q).toEqual({ limit: 50, offset: 0 })
    expect((q as { account?: string }).account).toBeUndefined()
  })

  it('includes user_id/kind/time when set', () => {
    expect(
      buildAdminBonusLedgerQuery({
        user_id: 7,
        kind: ' topup_first_bonus ',
        created_from: 100,
        created_to: '2026-01-01T00:00:00Z',
        limit: 20,
        offset: 40,
      })
    ).toEqual({
      limit: 20,
      offset: 40,
      user_id: 7,
      kind: 'topup_first_bonus',
      created_from: 100,
      created_to: '2026-01-01T00:00:00Z',
    })
  })

  it('omits non-positive user_id', () => {
    const q = buildAdminBonusLedgerQuery({ user_id: 0 })
    expect(q.user_id).toBeUndefined()
  })
})

describe('parseUserIdFilter', () => {
  it('parses positive integers only', () => {
    expect(parseUserIdFilter('12')).toBe(12)
    expect(parseUserIdFilter('')).toBeUndefined()
    expect(parseUserIdFilter('0')).toBeUndefined()
    expect(parseUserIdFilter('1.5')).toBeUndefined()
  })
})
