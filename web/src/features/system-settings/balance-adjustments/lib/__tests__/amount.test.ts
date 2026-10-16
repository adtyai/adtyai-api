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
  adjustQuotaToUsd,
  adjustUsdToQuota,
  balanceAdjustErrorMessage,
} from '../amount'

describe('adjustUsdToQuota / adjustQuotaToUsd', () => {
  it('round-trips positive and negative USD', () => {
    const pos = adjustUsdToQuota(1.5)
    expect(pos).toBeGreaterThan(0)
    expect(Math.abs(adjustQuotaToUsd(pos) - 1.5)).toBeLessThan(1e-9)

    const neg = adjustUsdToQuota(-2)
    expect(neg).toBeLessThan(0)
    expect(Math.abs(adjustQuotaToUsd(neg) + 2)).toBeLessThan(1e-9)
  })

  it('zero stays zero', () => {
    expect(adjustUsdToQuota(0)).toBe(0)
    expect(adjustQuotaToUsd(0)).toBe(0)
  })
})

describe('balanceAdjustErrorMessage', () => {
  it('maps self_approve_forbidden', () => {
    expect(balanceAdjustErrorMessage('self_approve_forbidden', 'x')).toBe(
      'Self-approve forbidden when multiple admins'
    )
  })

  it('maps already_decided', () => {
    expect(balanceAdjustErrorMessage('already_decided', 'x')).toBe(
      'Request already decided'
    )
  })
})
