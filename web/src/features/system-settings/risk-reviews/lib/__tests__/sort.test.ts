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

import type { RiskReview } from '../../types'
import { formatUsd, sortRiskReviewsForDisplay } from '../sort'

function row(
  partial: Partial<RiskReview> & Pick<RiskReview, 'id' | 'status'>
): RiskReview {
  return {
    kind: 'first_topup_extra',
    user_id: 1,
    order_no: '',
    amount_usd: 0,
    reason: '',
    payload: '',
    resolved_by: 0,
    created_at: '',
    ...partial,
  }
}

describe('sortRiskReviewsForDisplay', () => {
  it('puts pending before resolved and keeps id desc within status', () => {
    const sorted = sortRiskReviewsForDisplay([
      row({ id: 1, status: 'approved' }),
      row({ id: 3, status: 'pending' }),
      row({ id: 2, status: 'rejected' }),
      row({ id: 4, status: 'pending' }),
    ])
    expect(sorted.map((r) => r.id)).toEqual([4, 3, 2, 1])
  })
})

describe('formatUsd', () => {
  it('formats two decimals', () => {
    expect(formatUsd(10)).toBe('$10.00')
    expect(formatUsd(null)).toBe('-')
  })
})
