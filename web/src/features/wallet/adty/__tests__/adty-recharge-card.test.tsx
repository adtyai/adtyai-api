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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { AdtyPendingBanner } from '../components/adty-pending-banner'
import { AdtyRechargeCard } from '../components/adty-recharge-card'
import { bonusRateFor, bonusUsdFor } from '../lib/bonus'
import type { AdtyOrder, AdtyTopupInfo } from '../types'

const api = vi.hoisted(() => ({ quoteAdtyTopup: vi.fn() }))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

const info: AdtyTopupInfo = {
  tiers: [
    { amount: 10, rate: 1 },
    { amount: 100, rate: 2.5 },
  ],
  bonus_rates: { '10': 1, '30': 1.5, '100': 2.5 },
  min_topup: 1,
  methods: [
    { id: 'usdt.trc20', kind: 'usdt', available: true },
    { id: 'usdt.xlayer', kind: 'usdt', available: false },
    { id: 'alipay', kind: 'epay', available: true },
  ],
  price_cny_per_usd: 7.3,
  usdt_order_minutes: 15,
  epay_order_minutes: 10,
  pending: null,
  compliance_confirmed: true,
}

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

describe('bonus tiers', () => {
  it('uses the largest tier not above the amount', () => {
    expect(bonusRateFor(9, info.bonus_rates)).toBe(0)
    expect(bonusRateFor(29, info.bonus_rates)).toBe(1)
    expect(bonusRateFor(150, info.bonus_rates)).toBe(2.5)
    expect(bonusUsdFor(150, info.bonus_rates)).toBe(3.75)
  })
})

describe('AdtyRechargeCard', () => {
  it('adds the promo bonus from the quote and passes the code to the payment', async () => {
    api.quoteAdtyTopup.mockImplementation(
      async (_amount: number, _method: string, promo: string) => ({
        available: true,
        quote: {
          face_usd: '100',
          bonus_rate: '2.5',
          bonus_usd: '2.5',
          credit_quota: 56250000,
          pay_amount: '100',
          pay_currency: 'USDT',
          price_cny_per_usd: '0',
          topup_group_ratio: '1',
          promo_code_id: promo ? 3 : 0,
          promo_bonus_usd: promo ? '10' : '0',
        },
      })
    )
    const user = userEvent.setup()
    const onPay = vi.fn()
    render(
      <AdtyRechargeCard
        info={info}
        loading={false}
        paying={false}
        onPay={onPay}
      />,
      { wrapper }
    )
    await user.click(screen.getByRole('button', { name: /\$100/ }))
    await user.click(screen.getByRole('button', { name: /TRC20/ }))
    await user.type(screen.getByLabelText('Promo code (optional)'), 'TEN')
    expect(await screen.findByTestId('adty-promo-bonus')).toHaveTextContent(
      '$10.00'
    )
    expect(screen.getByTestId('adty-credit')).toHaveTextContent('$112.50')
    const pay = screen.getByRole('button', { name: 'Pay now' })
    await waitFor(() => expect(pay).toBeEnabled())
    await user.click(pay)
    expect(onPay).toHaveBeenCalledWith(100, info.methods[0], 'TEN')
  })

  it('shows the server reason and blocks payment for an invalid promo code', async () => {
    api.quoteAdtyTopup.mockImplementation(
      async (_amount: number, _method: string, promo: string) => {
        if (promo) throw new Error('This promo code is not valid')
        return {
          available: true,
          quote: {
            face_usd: '10',
            bonus_rate: '1',
            bonus_usd: '0.1',
            credit_quota: 5050000,
            pay_amount: '10',
            pay_currency: 'USDT',
            price_cny_per_usd: '0',
            topup_group_ratio: '1',
            promo_code_id: 0,
            promo_bonus_usd: '0',
          },
        }
      }
    )
    const user = userEvent.setup()
    render(
      <AdtyRechargeCard
        info={info}
        loading={false}
        paying={false}
        onPay={() => {}}
      />,
      { wrapper }
    )
    await user.click(screen.getByRole('button', { name: /^\$10Bonus/ }))
    await user.click(screen.getByRole('button', { name: /TRC20/ }))
    await user.type(screen.getByLabelText('Promo code (optional)'), 'NOPE')
    expect(
      await screen.findByText('This promo code is not valid')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Promo code (optional)')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeDisabled()
  })

  it('shows a $150 custom amount with a $3.75 bonus from the server rates', async () => {
    const user = userEvent.setup()
    render(
      <AdtyRechargeCard
        info={info}
        loading={false}
        paying={false}
        onPay={() => {}}
      />,
      { wrapper }
    )
    await user.type(screen.getByLabelText('Custom amount (USD)'), '150')
    expect(screen.getByTestId('adty-credit')).toHaveTextContent('$153.75')
    expect(screen.getByTestId('adty-credit')).toHaveTextContent('$3.75')
  })

  it('shows the CNY price and rate after choosing Alipay', async () => {
    api.quoteAdtyTopup.mockResolvedValue({
      available: true,
      quote: {
        face_usd: '100',
        bonus_rate: '2.5',
        bonus_usd: '2.5',
        credit_quota: 51250000,
        pay_amount: '730',
        pay_currency: 'CNY',
        price_cny_per_usd: '7.3',
        topup_group_ratio: '1',
        promo_code_id: 0,
        promo_bonus_usd: '0',
      },
    })
    const user = userEvent.setup()
    const onPay = vi.fn()
    render(
      <AdtyRechargeCard
        info={info}
        loading={false}
        paying={false}
        onPay={onPay}
      />,
      { wrapper }
    )
    await user.click(screen.getByRole('button', { name: /\$100/ }))
    await user.click(screen.getByRole('button', { name: /Alipay/ }))
    await waitFor(() =>
      expect(screen.getByTestId('adty-pay')).toHaveTextContent(
        '¥730.00 (1 USD = 7.30 CNY)'
      )
    )
    await user.click(screen.getByRole('button', { name: 'Pay now' }))
    expect(onPay).toHaveBeenCalledWith(100, info.methods[2], '')
  })

  it('shows plain amounts without bonus text when no bonus tiers are set', async () => {
    const user = userEvent.setup()
    render(
      <AdtyRechargeCard
        info={{
          ...info,
          tiers: [
            { amount: 10, rate: 0 },
            { amount: 100, rate: 0 },
          ],
          bonus_rates: {},
        }}
        loading={false}
        paying={false}
        onPay={() => {}}
      />,
      { wrapper }
    )
    expect(screen.queryByText('No bonus')).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Custom amount (USD)'), '150')
    expect(screen.getByTestId('adty-credit')).toHaveTextContent('$150.00')
    expect(screen.getByTestId('adty-credit')).not.toHaveTextContent('bonus')
  })

  it('disables a method that is unavailable', () => {
    render(
      <AdtyRechargeCard
        info={info}
        loading={false}
        paying={false}
        onPay={() => {}}
      />,
      { wrapper }
    )
    expect(screen.getByRole('button', { name: /X Layer/ })).toBeDisabled()
  })

  it('opens top-up without a compliance confirmation', () => {
    // adtyai dropped the payment compliance step on 2026-11-01.
    render(
      <AdtyRechargeCard
        info={{ ...info, compliance_confirmed: false }}
        loading={false}
        paying={false}
        onPay={() => {}}
      />,
      { wrapper }
    )
    expect(screen.queryByText('Top-up is not open yet')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeInTheDocument()
  })
})

describe('AdtyPendingBanner', () => {
  it('reopens the payment of a pending order', async () => {
    const order = {
      trade_no: 'EPU1',
      payment_method: 'usdt.trc20',
      payment_provider: 'bepusdt',
      status: 'pending',
      amount: 100,
      ext: { face_usd: '100' },
    } as AdtyOrder
    const onResume = vi.fn()
    const user = userEvent.setup()
    render(<AdtyPendingBanner order={order} onResume={onResume} />)
    await user.click(screen.getByRole('button', { name: 'Continue payment' }))
    expect(onResume).toHaveBeenCalledWith(order)
  })

  it('renders nothing without a pending order', () => {
    const { container } = render(
      <AdtyPendingBanner order={null} onResume={() => {}} />
    )
    expect(container).toBeEmptyDOMElement()
  })
})
