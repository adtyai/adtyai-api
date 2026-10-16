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

import type { AdtyCreateTopupOrderData, AdtyTopupTier } from '../../types'
import {
  computeAdtyBonusPlan,
  computePayLocalAmount,
  firstTopupBonusRateToPercent,
  formatCountdown,
  formatUsdAmount,
  isFixedTierAmount,
  remainingSecondsUntil,
  resolveAdtyCheckoutLaunch,
} from '../adty-topup'

const tiers: AdtyTopupTier[] = [
  { amount: 10, bonus_rate: 0.01, bonus_percent: 1, bonus_amount: 0.1 },
  { amount: 100, bonus_rate: 0.025, bonus_percent: 2.5, bonus_amount: 2.5 },
  { amount: 1000, bonus_rate: 0.04, bonus_percent: 4, bonus_amount: 40 },
]

describe('adty-topup helpers', () => {
  it('converts first_topup_bonus_rate fraction and percent', () => {
    expect(firstTopupBonusRateToPercent(0.1)).toBe(10)
    expect(firstTopupBonusRateToPercent(0.15)).toBe(15)
    expect(firstTopupBonusRateToPercent(10)).toBe(10)
    expect(firstTopupBonusRateToPercent(null)).toBe(10)
  })

  it('accepts only fixed tier amounts', () => {
    expect(isFixedTierAmount(100, tiers)).toBe(true)
    expect(isFixedTierAmount(99, tiers)).toBe(false)
    expect(isFixedTierAmount(0, tiers)).toBe(false)
  })

  it('takes max of first vs tier when eligible', () => {
    const plan = computeAdtyBonusPlan(100, tiers[1], 0.1, true)
    expect(plan).not.toBeNull()
    expect(plan?.chosenBonusUSD).toBe(10)
    expect(plan?.usedFirst).toBe(true)
    expect(plan?.creditTotalUSD).toBe(110)
  })

  it('uses tier only when not first-eligible', () => {
    const plan = computeAdtyBonusPlan(100, tiers[1], 0.1, false)
    expect(plan?.chosenBonusUSD).toBe(2.5)
    expect(plan?.usedFirst).toBe(false)
    expect(plan?.creditTotalUSD).toBe(102.5)
  })

  it('computes local pay via Price', () => {
    expect(computePayLocalAmount(1000, 7.3)).toBe(7300)
    expect(computePayLocalAmount(10, 7.3)).toBe(73)
  })

  it('formats USD with a plain $ prefix (not US$)', () => {
    expect(formatUsdAmount(100)).toBe('$100')
    expect(formatUsdAmount(10.5)).toBe('$10.50')
    expect(formatUsdAmount(100000)).toBe('$100,000')
    expect(formatUsdAmount(Number.NaN)).toBe('-')
  })
})

describe('USDT checkout helpers', () => {
  it('formats countdown as M:SS', () => {
    expect(formatCountdown(0)).toBe('0:00')
    expect(formatCountdown(65)).toBe('1:05')
    expect(formatCountdown(11 * 60 + 24)).toBe('11:24')
  })

  it('computes remaining seconds until expire_at', () => {
    const now = 1_700_000_000_000
    expect(remainingSecondsUntil(1_700_000_000 + 90, now)).toBe(90)
    expect(remainingSecondsUntil(1_700_000_000 - 10, now)).toBe(0)
  })

  it('opens usdt cashier when data.usdt present (no false fail)', () => {
    const data: AdtyCreateTopupOrderData = {
      order: { order_no: 'TP1', status: 'pending', amount_usd: 100 },
      usdt: {
        payment_address: 'TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE',
        payment_amount: '100.0137',
        pay_usdt: 100.0137,
        expire_at: 1_700_000_100,
        usdt_fx_rate: 1.000137,
      },
    }
    const launch = resolveAdtyCheckoutLaunch(data)
    expect(launch.kind).toBe('usdt')
    if (launch.kind === 'usdt') {
      expect(launch.usdt.payment_address).toMatch(/^T/)
      expect(launch.usdt.payment_amount).toBe('100.0137')
    }
  })

  it('still launches epay via pay_url', () => {
    const data: AdtyCreateTopupOrderData = {
      order: { order_no: 'TP2', status: 'pending' },
      pay_url: 'https://pay.example/checkout',
      pay_params: { pid: '1' },
    }
    const launch = resolveAdtyCheckoutLaunch(data)
    expect(launch.kind).toBe('epay')
    if (launch.kind === 'epay') {
      expect(launch.payUrl).toContain('pay.example')
    }
  })

  it('fails only when neither usdt nor pay_url', () => {
    expect(resolveAdtyCheckoutLaunch({ order: { order_no: 'TP3' } }).kind).toBe(
      'fail'
    )
  })
})
