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
import type { AdtyMethodKind } from '../types'

/** The payment the wallet is running: a new order, or an existing one to resume. */
export type ActivePayment = {
  kind: AdtyMethodKind
  method: string
  amount: number
  /** Promo code applied at order time (adty task 12.4); '' for none. */
  promoCode?: string
  /** Set when resuming an order that already exists. */
  tradeNo?: string
}
