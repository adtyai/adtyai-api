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

/** Official DEC-025 lifecycle states. A commission row may be split across several. */
export const OFFICIAL_COMMISSION_STATE_LABELS = {
  on_hold: 'On hold',
  pending_coverage: 'Pending coverage',
  converted: 'Converted to balance',
  withdrawal_requested: 'Withdrawal requested',
  withdrawn_released: 'External payout completed and released',
  not_issued: 'Not issued',
} as const

export type OfficialCommissionState = keyof typeof OFFICIAL_COMMISSION_STATE_LABELS

export type OfficialCommissionStateAmount = {
  state: string
  amount_usd: string
}

export type CommissionAmountFields = {
  amount_usd?: string
  clawed_usd?: string
  converted_usd?: string
  converted_clawed_usd?: string
  withdrawn_usd?: string
  frozen_usd?: string
  earnings_hold_usd?: string
  margin_hold_usd?: string
  official_states?: OfficialCommissionStateAmount[]
}

function decimalOrZero(raw?: string) {
  const value = Number(raw ?? 0)
  return Number.isFinite(value) ? value : 0
}

function formatAmount(value: number) {
  if (!Number.isFinite(value) || value === 0) return '0'
  return String(value)
}

/** Derives official states from the row when the API has not already supplied them. */
export function officialCommissionStates(
  row: CommissionAmountFields,
  withdrawnLiveUsd?: string
): OfficialCommissionStateAmount[] {
  if (row.official_states && row.official_states.length > 0) {
    return row.official_states
  }
  const hold = decimalOrZero(row.earnings_hold_usd) + decimalOrZero(row.margin_hold_usd)
  const pendingWithdrawal = Math.max(decimalOrZero(row.frozen_usd), 0)
  const onHold = Math.max(hold - pendingWithdrawal, 0)
  const convertedLive = Math.max(
    decimalOrZero(row.converted_usd) - decimalOrZero(row.converted_clawed_usd),
    0
  )
  const withdrawnLive =
    withdrawnLiveUsd === undefined
      ? Math.max(decimalOrZero(row.withdrawn_usd), 0)
      : Math.max(decimalOrZero(withdrawnLiveUsd), 0)
  const uncovered = Math.max(
    decimalOrZero(row.amount_usd) -
      decimalOrZero(row.clawed_usd) -
      hold -
      convertedLive -
      withdrawnLive,
    0
  )
  const states: OfficialCommissionStateAmount[] = []
  const append = (state: OfficialCommissionState, amount: number) => {
    if (amount > 0) states.push({ state, amount_usd: formatAmount(amount) })
  }
  append('on_hold', onHold)
  append('pending_coverage', uncovered)
  append('converted', convertedLive)
  append('withdrawal_requested', pendingWithdrawal)
  append('withdrawn_released', Math.max(decimalOrZero(row.withdrawn_usd), 0))
  return states
}

export function officialCommissionStateLabel(state: string) {
  if (state in OFFICIAL_COMMISSION_STATE_LABELS) {
    return OFFICIAL_COMMISSION_STATE_LABELS[state as OfficialCommissionState]
  }
  return state
}

/** Convert-to-balance stays limited to rows already marked available. */
export function canConvertCommissionToBalance(status: string) {
  return status === 'available'
}
