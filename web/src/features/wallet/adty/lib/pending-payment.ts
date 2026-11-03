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
// Ported from the previous fork: remembers the order the user is paying so
// the wallet can reopen its payment dialog. The record expires with the order.
export const PENDING_PAYMENT_STORAGE_KEY = 'adty.pending-payment'

export interface PendingPayment {
  tradeNo: string
  method: string
  /** Unix seconds from the server; the record is dropped after it. */
  expireAt: number
}

function isPendingPayment(value: unknown): value is PendingPayment {
  if (!value || typeof value !== 'object') return false
  const payment = value as Record<string, unknown>
  return (
    typeof payment.tradeNo === 'string' &&
    payment.tradeNo.trim() !== '' &&
    typeof payment.method === 'string' &&
    typeof payment.expireAt === 'number' &&
    Number.isFinite(payment.expireAt)
  )
}

export function savePendingPayment(storage: Storage, payment: PendingPayment) {
  storage.setItem(PENDING_PAYMENT_STORAGE_KEY, JSON.stringify(payment))
}

export function clearPendingPayment(storage: Storage) {
  storage.removeItem(PENDING_PAYMENT_STORAGE_KEY)
}

export function loadPendingPayment(
  storage: Storage,
  nowSeconds = Math.floor(Date.now() / 1000)
): PendingPayment | null {
  const raw = storage.getItem(PENDING_PAYMENT_STORAGE_KEY)
  if (!raw) return null
  try {
    const payment: unknown = JSON.parse(raw)
    if (!isPendingPayment(payment) || payment.expireAt <= nowSeconds) {
      clearPendingPayment(storage)
      return null
    }
    return payment
  } catch {
    clearPendingPayment(storage)
    return null
  }
}
