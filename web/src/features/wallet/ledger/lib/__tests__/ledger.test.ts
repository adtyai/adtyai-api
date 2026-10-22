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
  LEDGER_KIND_OPTIONS,
  buildWalletLedgerSelfQuery,
  clampLedgerLimit,
  clampLedgerOffset,
  datetimeLocalToUnix,
  ledgerKindLabelKey,
  ledgerNote,
  splitAccountColumns,
} from '../ledger'

describe('clampLedgerLimit / offset', () => {
  it('defaults invalid or oversized limit to 50', () => {
    expect(clampLedgerLimit(undefined)).toBe(50)
    expect(clampLedgerLimit(0)).toBe(50)
    expect(clampLedgerLimit(101)).toBe(50)
    expect(clampLedgerLimit(20)).toBe(20)
  })

  it('clamps negative offset to 0', () => {
    expect(clampLedgerOffset(-1)).toBe(0)
    expect(clampLedgerOffset(10)).toBe(10)
  })
})

describe('buildWalletLedgerSelfQuery', () => {
  it('always sends limit and offset', () => {
    expect(buildWalletLedgerSelfQuery({})).toEqual({ limit: 50, offset: 0 })
  })

  it('includes account/kind/time only when set', () => {
    expect(
      buildWalletLedgerSelfQuery({
        account: 'paid',
        kind: ' topup_paid ',
        created_from: 100,
        created_to: 200,
        limit: 20,
        offset: 40,
      })
    ).toEqual({
      limit: 20,
      offset: 40,
      account: 'paid',
      kind: 'topup_paid',
      created_from: 100,
      created_to: 200,
    })
  })

  it('omits empty account (both paid+bonus)', () => {
    const q = buildWalletLedgerSelfQuery({ account: '' })
    expect(q.account).toBeUndefined()
  })
})

describe('splitAccountColumns', () => {
  it('puts delta on paid column only', () => {
    expect(splitAccountColumns({ account: 'paid', delta: 100 })).toEqual({
      paidDelta: 100,
      bonusDelta: null,
    })
    expect(splitAccountColumns({ account: 'quota', delta: -5 })).toEqual({
      paidDelta: -5,
      bonusDelta: null,
    })
  })

  it('puts delta on bonus column only', () => {
    expect(splitAccountColumns({ account: 'bonus', delta: 50 })).toEqual({
      paidDelta: null,
      bonusDelta: 50,
    })
  })

  it('unknown account leaves both empty', () => {
    expect(splitAccountColumns({ account: 'aff', delta: 1 })).toEqual({
      paidDelta: null,
      bonusDelta: null,
    })
  })
})

describe('ledgerKindLabelKey', () => {
  it('maps known kinds', () => {
    expect(ledgerKindLabelKey('topup_paid')).toBe('Ledger kind topup paid')
    expect(ledgerKindLabelKey('api_consume_hourly')).toBe(
      'Ledger kind api consume'
    )
  })

  it('names affiliate refund kinds that can appear in the ledger', () => {
    expect(LEDGER_KIND_OPTIONS).toEqual(
      expect.arrayContaining([
        'aff_refund_void',
        'aff_refund_claw',
        'aff_refund_claw_paid',
      ])
    )
    expect(ledgerKindLabelKey('aff_refund_void')).toBe(
      'Ledger kind aff refund void'
    )
    expect(ledgerKindLabelKey('aff_refund_claw')).toBe(
      'Ledger kind aff refund claw'
    )
    expect(ledgerKindLabelKey('aff_refund_claw_paid')).toBe(
      'Ledger kind aff refund claw paid'
    )
  })

  it('filters and labels the kinds the server actually writes', () => {
    expect(LEDGER_KIND_OPTIONS).toEqual(
      expect.arrayContaining(['redemption_bonus', 'signup_invite_bonus'])
    )
    expect(LEDGER_KIND_OPTIONS).not.toContain('redemption')
    expect(ledgerKindLabelKey('redemption_bonus')).toBe(
      'Ledger kind redemption'
    )
    expect(ledgerKindLabelKey('signup_invite_bonus')).toBe(
      'Ledger kind signup invite bonus'
    )
    expect(
      ledgerNote(
        { reason: 'invitation code bonus', ref_id: '7' },
        (key) => `T:${key}`
      )
    ).toBe('T:Invitation code bonus · 7')
  })
})

describe('datetimeLocalToUnix / note', () => {
  it('parses datetime-local', () => {
    expect(datetimeLocalToUnix('')).toBeUndefined()
    const u = datetimeLocalToUnix('2026-10-15T16:00')
    expect(typeof u).toBe('number')
    expect(u).toBeGreaterThan(0)
  })

  it('builds note from reason and ref', () => {
    expect(ledgerNote({ reason: 'a', ref_id: 'b' })).toBe('a · b')
    expect(ledgerNote({ reason: '', ref_id: '' })).toBe('—')
  })
})
