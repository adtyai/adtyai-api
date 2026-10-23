/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { AdtyOrderList } from '../adty-order-list'

const listAdtyTopupOrders = vi.fn()

vi.mock('../../api', () => ({
  listAdtyTopupOrders: (...args: unknown[]) => listAdtyTopupOrders(...args),
  continueAdtyTopupOrder: vi.fn(),
}))

vi.mock('../adty-usdt-cashier-dialog', () => ({
  AdtyUsdtCashierDialog: () => null,
}))

describe('AdtyOrderList money visibility', () => {
  beforeEach(() => {
    listAdtyTopupOrders.mockReset()
  })

  it('shows refund state and paid/bonus split instead of only Credited', async () => {
    listAdtyTopupOrders.mockResolvedValue({
      success: true,
      data: {
        page: 1,
        total: 1,
        items: [
          {
            order_no: 'ADTY-REFUND-1',
            amount_usd: 10,
            paid_amount_usd: 10,
            credit_amount_usd: 10,
            bonus_amount_usd: 0.5,
            refunded_amount: 4,
            payment_method: 'alipay',
            status: 'paid',
            display_status: 'paid',
            refund_status: 'partial',
            can_continue: false,
            create_time: 1700000000,
          },
        ],
      },
    })

    render(<AdtyOrderList />)

    expect(await screen.findByText(/ADTY-REFUND-1/)).toBeVisible()
    expect(screen.getByText(/Refund partial/)).toBeVisible()
    expect(screen.getByText(/Credit amount/)).toBeVisible()
    expect(screen.getByText(/Bonus amount/)).toBeVisible()
    expect(screen.getByText(/Already refunded/)).toBeVisible()
    expect(screen.getByText(/\$0\.50/)).toBeVisible()
    expect(screen.getByText(/\$4/)).toBeVisible()
  })
})
