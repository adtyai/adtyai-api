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
import type { ReferralWithdrawal } from '@/features/referral/types'

/** What the admin must pay: USDT net of the fee, CNY at the locked rate, or nothing for balance. */
export function payoutText(row: ReferralWithdrawal, usd: string): string {
  if (row.account_type === 'usdt') return `${row.net_amount.toFixed(2)} USDT`
  if (row.cny_amount) return `¥${row.cny_amount} (1 USD = ${row.cny_rate} CNY)`
  return usd
}
