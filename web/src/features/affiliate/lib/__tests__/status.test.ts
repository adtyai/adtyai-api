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

import { adminActionsForWithdrawStatus, canRetryWithdraw } from '../status'

describe('aff withdraw status machine UI', () => {
  it('pending 鈫?approve/reject', () => {
    expect(adminActionsForWithdrawStatus('pending')).toEqual([
      'approve',
      'reject',
    ])
  })

  it('approved 鈫?mark paid/failed/unknown', () => {
    expect(adminActionsForWithdrawStatus('approved')).toEqual([
      'mark-paid',
      'mark-failed',
      'mark-unknown',
    ])
  })

  it('pay_failed allows retry; pay_unknown and final do not', () => {
    expect(adminActionsForWithdrawStatus('pay_failed')).toEqual(['retry'])
    expect(canRetryWithdraw('pay_failed')).toBe(true)
    expect(adminActionsForWithdrawStatus('pay_unknown')).toEqual([
      'mark-paid',
      'mark-failed',
    ])
    expect(canRetryWithdraw('pay_unknown')).toBe(false)
    expect(adminActionsForWithdrawStatus('pay_failed_final')).toEqual([])
    expect(canRetryWithdraw('pay_failed_final')).toBe(false)
  })

  it('terminal states have no actions', () => {
    expect(adminActionsForWithdrawStatus('paid')).toEqual([])
    expect(adminActionsForWithdrawStatus('rejected')).toEqual([])
  })
})
