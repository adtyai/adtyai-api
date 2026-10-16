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
import { beforeEach, describe, expect, it } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  affQuotaToUsd,
  affUsdToQuota,
  estimateWithdrawFeeQuota,
  newWithdrawIdemKey,
} from '../quota'

describe('aff quota helpers', () => {
  beforeEach(() => {
    useSystemConfigStore.setState({
      config: {
        currency: {
          quotaDisplayType: 'USD',
          quotaPerUnit: 500000,
          usdExchangeRate: 1,
          customCurrencyExchangeRate: 1,
          customCurrencySymbol: '¤',
          price: 1,
        },
      } as never,
    })
  })

  it('converts quota ↔ USD with quotaPerUnit', () => {
    expect(affQuotaToUsd(500000)).toBe(1)
    expect(affUsdToQuota(20)).toBe(10_000_000)
  })

  it('estimates fee like backend floor', () => {
    const { feeQuota, netQuota } = estimateWithdrawFeeQuota(10_000_000, 0.01)
    expect(feeQuota).toBe(100_000)
    expect(netQuota).toBe(9_900_000)
  })

  it('generates unique idem keys', () => {
    const a = newWithdrawIdemKey()
    const b = newWithdrawIdemKey()
    expect(a).toBeTruthy()
    expect(a).not.toBe(b)
  })
})
