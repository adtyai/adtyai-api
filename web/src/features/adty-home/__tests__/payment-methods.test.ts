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

import { paymentMethodSentence } from '../lib/payment-methods'

const t = ((key: string, values?: { methods?: string }) => {
  if (values?.methods) return key.replace('{{methods}}', values.methods)
  return key
}) as never

describe('landing payment methods', () => {
  it('does not name a method that the platform has not enabled', () => {
    expect(paymentMethodSentence(['alipay'], t)).toBe('Top up with Alipay.')
    expect(paymentMethodSentence(['alipay'], t)).not.toMatch(/WeChat|USDT/)
    expect(paymentMethodSentence([], t)).toMatch(/No payment methods available/)
    expect(paymentMethodSentence(undefined, t)).toMatch(
      /payment methods currently enabled/
    )
  })
})
