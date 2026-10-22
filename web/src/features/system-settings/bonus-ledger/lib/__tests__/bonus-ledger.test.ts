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

import { BONUS_LEDGER_KIND_OPTIONS } from '../bonus-ledger'

describe('BONUS_LEDGER_KIND_OPTIONS', () => {
  it('only lists kinds that can be written to the bonus account', () => {
    expect(BONUS_LEDGER_KIND_OPTIONS).toEqual([
      'topup_tier_bonus',
      'topup_first_bonus',
      'api_consume_hourly',
      'topup_refund_bonus_claw',
      'balance_adjust',
      'redemption_bonus',
      'signup_invite_bonus',
      'checkin_bonus',
    ])
    expect(BONUS_LEDGER_KIND_OPTIONS).not.toContain('topup_paid')
    expect(BONUS_LEDGER_KIND_OPTIONS).not.toContain('aff_refund_void')
    expect(BONUS_LEDGER_KIND_OPTIONS).not.toContain('aff_refund_claw')
    expect(BONUS_LEDGER_KIND_OPTIONS).not.toContain('aff_refund_claw_paid')
  })
})
