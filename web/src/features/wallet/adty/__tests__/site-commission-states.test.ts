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
  canConvertCommissionToBalance,
  officialCommissionStateLabel,
  officialCommissionStates,
  OFFICIAL_COMMISSION_STATE_LABELS,
} from '../commission-states'

describe('official commission states', () => {
  it('lists official DEC-025 names and hides covered as a product state', () => {
    expect(OFFICIAL_COMMISSION_STATE_LABELS.on_hold).toBe('On hold')
    expect(OFFICIAL_COMMISSION_STATE_LABELS.pending_coverage).toBe('Pending coverage')
    expect(OFFICIAL_COMMISSION_STATE_LABELS.converted).toBe('Converted to balance')
    expect(OFFICIAL_COMMISSION_STATE_LABELS.withdrawal_requested).toBe(
      'Withdrawal requested'
    )
    expect(OFFICIAL_COMMISSION_STATE_LABELS.withdrawn_released).toBe(
      'External payout completed and released'
    )
    expect(OFFICIAL_COMMISSION_STATE_LABELS.not_issued).toBe('Not issued')
    // 断言「不存在 Covered 这个标签」。labels 是 as const，Object.values 推出
    // 字面量联合，直接 includes 一个不在联合里的字符串会被类型系统拒绝；
    // 这里刻意放宽成 string[]，因为要证明的正是「它不在里面」。
    expect(
      (Object.values(OFFICIAL_COMMISSION_STATE_LABELS) as string[]).includes(
        'Covered'
      )
    ).toBe(false)
    expect(officialCommissionStateLabel('covered')).toBe('covered')
  })

  it('itemizes a split row under official states', () => {
    const states = officialCommissionStates({
      amount_usd: '10',
      earnings_hold_usd: '3',
      margin_hold_usd: '1',
      frozen_usd: '1',
      converted_usd: '3',
      converted_clawed_usd: '0',
      withdrawn_usd: '2',
      clawed_usd: '0',
    })
    expect(states).toEqual([
      { state: 'on_hold', amount_usd: '3' },
      { state: 'pending_coverage', amount_usd: '1' },
      { state: 'converted', amount_usd: '3' },
      { state: 'withdrawal_requested', amount_usd: '1' },
      { state: 'withdrawn_released', amount_usd: '2' },
    ])
    expect(canConvertCommissionToBalance('available')).toBe(true)
    expect(canConvertCommissionToBalance('pending_coverage')).toBe(false)
    expect(canConvertCommissionToBalance('covered')).toBe(false)
  })
})
