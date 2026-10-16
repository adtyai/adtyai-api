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
import type {
  AdtyCreateTopupOrderData,
  AdtyPayMethod,
  AdtyTopupOrder,
  AdtyTopupTier,
  AdtyUsdtCashier,
} from '../types'

function toFiniteNumber(rate: unknown): number | null {
  if (typeof rate === 'number') {
    return Number.isFinite(rate) ? rate : null
  }
  if (typeof rate === 'string') {
    const n = Number(rate)
    return Number.isFinite(n) ? n : null
  }
  return null
}

/** Convert FirstTopupBonusRate fraction (0.10) or percent (10) to display percent. */
export function firstTopupBonusRateToPercent(rate: unknown): number {
  const n = toFiniteNumber(rate)
  if (n == null || n < 0) return 10
  const percent = n <= 1 ? n * 100 : n
  return Math.round(percent * 100) / 100
}

/** Normalize rate to 0–1 fraction. */
export function toBonusFraction(rate: unknown): number {
  const n = toFiniteNumber(rate)
  if (n == null || n < 0) return 0.1
  if (n > 1) return n / 100
  return n
}

export function isFixedTierAmount(
  amount: number,
  tiers: AdtyTopupTier[]
): boolean {
  if (!Number.isFinite(amount) || amount <= 0) return false
  return tiers.some((t) => Number(t.amount) === amount)
}

export type AdtyBonusPlan = {
  tierRate: number
  firstRate: number
  tierPercent: number
  firstPercent: number
  tierBonusUSD: number
  firstBonusUSD: number
  chosenBonusUSD: number
  chosenPercent: number
  usedFirst: boolean
  creditTotalUSD: number
}

function tierBonusRate(tier: AdtyTopupTier): number {
  if (typeof tier.bonus_rate === 'number' && Number.isFinite(tier.bonus_rate)) {
    if (tier.bonus_rate > 1) return tier.bonus_rate / 100
    return tier.bonus_rate
  }
  if (typeof tier.bonus_percent === 'number') {
    return tier.bonus_percent / 100
  }
  return 0
}

/**
 * Mirror backend ComputeBonusPlan display: when first-topup eligible,
 * take max(tier, first); otherwise tier only.
 */
export function computeAdtyBonusPlan(
  amountUSD: number,
  tier: AdtyTopupTier | undefined,
  firstTopupBonusRate: number,
  firstEligible: boolean
): AdtyBonusPlan | null {
  if (!tier || !Number.isFinite(amountUSD) || amountUSD <= 0) return null

  const tierRate = tierBonusRate(tier)
  const firstRate = toBonusFraction(firstTopupBonusRate)
  const tierPercent = Math.round(tierRate * 10000) / 100
  const firstPercent = firstTopupBonusRateToPercent(firstRate)

  let tierBonusUSD: number
  if (typeof tier.bonus_amount === 'number' && Number.isFinite(tier.bonus_amount)) {
    tierBonusUSD = tier.bonus_amount
  } else {
    tierBonusUSD = Math.round(amountUSD * tierRate * 10000) / 10000
  }
  const firstBonusUSD = Math.round(amountUSD * firstRate * 10000) / 10000

  let chosenBonusUSD = tierBonusUSD
  let chosenPercent = tierPercent
  let usedFirst = false
  if (firstEligible && firstBonusUSD > tierBonusUSD) {
    chosenBonusUSD = firstBonusUSD
    chosenPercent = firstPercent
    usedFirst = true
  } else if (
    firstEligible &&
    firstBonusUSD === tierBonusUSD &&
    firstRate >= tierRate
  ) {
    usedFirst = true
    chosenPercent = firstPercent
  }

  return {
    tierRate,
    firstRate,
    tierPercent,
    firstPercent,
    tierBonusUSD,
    firstBonusUSD,
    chosenBonusUSD,
    chosenPercent,
    usedFirst,
    creditTotalUSD: Math.round((amountUSD + chosenBonusUSD) * 100) / 100,
  }
}

/** Local-currency pay amount = USD tier × Price (status.price). */
export function computePayLocalAmount(
  amountUSD: number,
  price: number
): number {
  const p = Number.isFinite(price) && price > 0 ? price : 1
  return Math.round(amountUSD * p * 100) / 100
}

export function findTier(
  tiers: AdtyTopupTier[],
  amount: number
): AdtyTopupTier | undefined {
  return tiers.find((t) => Number(t.amount) === amount)
}

export function pickDefaultPayMethod(
  methods: AdtyPayMethod[]
): AdtyPayMethod | undefined {
  return methods.find((m) => m.available) ?? methods[0]
}

/** Display USD with a plain `$` prefix (never locale `US$`). */
export function formatUsdAmount(amount: number): string {
  if (!Number.isFinite(amount)) return '-'
  const abs = Math.abs(amount)
  const digits = abs % 1 === 0 ? 0 : 2
  const body = abs.toLocaleString('en-US', {
    minimumFractionDigits: digits,
    maximumFractionDigits: 2,
  })
  return amount < 0 ? `-$${body}` : `$${body}`
}


// ============================================================================
// USDT cashier helpers
// ============================================================================

export type AdtyCheckoutLaunch =
  | { kind: 'usdt'; usdt: AdtyUsdtCashier; order: AdtyTopupOrder }
  | {
      kind: 'epay'
      payUrl: string
      payParams?: Record<string, string>
    }
  | { kind: 'fail' }

/**
 * Prefer USDT cashier when present; else epay pay_url; else fail.
 * Avoids false "Failed to open payment checkout" for USDT.
 */
export function resolveAdtyCheckoutLaunch(
  data?: AdtyCreateTopupOrderData | null
): AdtyCheckoutLaunch {
  if (!data?.order) return { kind: 'fail' }
  if (
    data.usdt?.payment_address &&
    (data.usdt.payment_amount || data.usdt.pay_usdt)
  ) {
    return { kind: 'usdt', usdt: data.usdt, order: data.order }
  }
  if (data.pay_url) {
    return {
      kind: 'epay',
      payUrl: data.pay_url,
      payParams: data.pay_params,
    }
  }
  return { kind: 'fail' }
}

/** Display/copy amount: prefer payment_amount string, else pay_usdt. */
export function formatUsdtPayAmount(usdt: AdtyUsdtCashier): string {
  if (
    usdt.payment_amount != null &&
    String(usdt.payment_amount).trim() !== ''
  ) {
    return String(usdt.payment_amount).trim()
  }
  if (typeof usdt.pay_usdt === 'number' && Number.isFinite(usdt.pay_usdt)) {
    return String(usdt.pay_usdt)
  }
  return ''
}

/** Seconds remaining until expire_at (unix seconds). Clamped at 0. */
export function remainingSecondsUntil(
  expireAtUnix: number,
  nowMs: number = Date.now()
): number {
  if (!Number.isFinite(expireAtUnix) || expireAtUnix <= 0) return 0
  const left = Math.floor(expireAtUnix - nowMs / 1000)
  return left > 0 ? left : 0
}

/** Format seconds as M:SS or MM:SS. */
export function formatCountdown(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const m = Math.floor(s / 60)
  const r = s % 60
  return `${m}:${r.toString().padStart(2, '0')}`
}

export function isTerminalTopupStatus(status: string | undefined): boolean {
  return status === 'paid' || status === 'failed' || status === 'need_manual'
}
