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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { Wallet } from '../../index'
import { AdtyPaymentDialogs } from '../components/adty-payment-dialogs'
import { AdtyRechargeCard } from '../components/adty-recharge-card'
import type { AdtyOrder, AdtyTopupInfo } from '../types'

const clients: QueryClient[] = []
function mount(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  )
}
const info: AdtyTopupInfo = {
  tiers: [],
  bonus_rates: {},
  min_topup: 10,
  methods: [
    {
      id: 'creem:small',
      kind: 'hosted',
      label: 'Small credit pack',
      face_usd: 2.5,
      available: true,
    },
  ],
  price_cny_per_usd: 7,
  usdt_order_minutes: 15,
  epay_order_minutes: 10,
  pending: null,
  compliance_confirmed: true,
}
function order(
  status: AdtyOrder['status'] = 'pending',
  payUrl = 'https://checkout.example/pay'
): AdtyOrder {
  return {
    id: 1,
    user_id: 1,
    trade_no: 'HOST1',
    payment_method: 'stripe',
    payment_provider: 'stripe',
    status,
    create_time: 1,
    complete_time: 0,
    amount: 10,
    money: 9,
    ext: {
      trade_no: 'HOST1',
      provider: 'stripe',
      method: 'stripe',
      face_usd: '10',
      bonus_rate: '0',
      bonus_usd: '0',
      promo_bonus_usd: '0',
      credit_quota: 5000000,
      pay_amount: '9',
      pay_currency: 'EUR',
      price_snapshot: '',
      expire_at: Math.floor(Date.now() / 1000) + 600,
      gateway_trade_id: '',
      pay_address: '',
      pay_url: payUrl,
      qr_code: '',
      url_scheme: '',
      channel_id: 0,
      paid_amount: '',
      paid_currency: '',
      paid_tx_hash: '',
      refunded_at: 0,
    },
  }
}
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  localStorage.clear()
})

it('uses a hosted registry label and fractional fixed face value, and renders the quoted ISO currency', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        available: true,
        quote: {
          face_usd: '2.5',
          bonus_rate: '0',
          bonus_usd: '0',
          promo_bonus_usd: '0',
          credit_quota: 1250000,
          pay_amount: '2.3',
          pay_currency: 'EUR',
        },
      },
    },
  })
  const onPay = vi.fn()
  mount(
    <AdtyRechargeCard
      info={info}
      loading={false}
      paying={false}
      onPay={onPay}
    />
  )
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Small credit pack' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/adty/topup/quote', {
      amount: 2.5,
      method: 'creem:small',
      promo_code: '',
    })
  )
  expect(await screen.findByTestId('adty-pay')).toHaveTextContent('€2.30')
  expect(screen.getByTestId('adty-credit')).toHaveTextContent('$2.50')
  await userEvent.setup().click(screen.getByRole('button', { name: 'Pay now' }))
  expect(onPay).toHaveBeenCalledWith(2.5, info.methods[0], '')
})

it('shows a non-promo quote failure and blocks payment', async () => {
  vi.spyOn(api, 'post').mockRejectedValue(
    new Error('Minimum for this provider is 20 USD')
  )
  mount(
    <AdtyRechargeCard
      info={{
        ...info,
        min_topup: 1,
        methods: [
          { id: 'stripe', kind: 'hosted', label: 'Stripe', available: true },
        ],
      }}
      loading={false}
      paying={false}
      onPay={() => {}}
    />
  )
  await userEvent
    .setup()
    .type(screen.getByLabelText('Custom amount (USD)'), '10')
  await userEvent.setup().click(screen.getByRole('button', { name: 'Stripe' }))
  expect(
    await screen.findByText('Minimum for this provider is 20 USD')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Pay now' })).toBeDisabled()
})

it('creates through the hosted endpoint, polls after returning, and refreshes credited funds once', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      message: 'success',
      data: {
        order_id: 'HOST1',
        checkout_url: 'https://checkout.example/pay',
      },
    },
  })
  let status: AdtyOrder['status'] = 'pending'
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: order(status) },
  }))
  const onSettled = vi.fn()
  mount(
    <AdtyPaymentDialogs
      payment={{
        kind: 'hosted',
        method: 'stripe',
        amount: 10,
        promoCode: 'TEN',
      }}
      onClose={() => {}}
      onSettled={onSettled}
    />
  )
  const link = await screen.findByRole('link', { name: 'Continue payment' })
  expect(link).toHaveAttribute('href', 'https://checkout.example/pay')
  expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  expect(post).toHaveBeenCalledExactlyOnceWith('/api/adty/topup/hosted/pay', {
    amount: 10,
    method: 'stripe',
    promo_code: 'TEN',
  })
  expect(await screen.findByText('€9.00')).toBeVisible()
  status = 'success'
  fireEvent(document, new Event('visibilitychange'))
  expect(await screen.findByText('Credited $10.00')).toBeVisible()
  await waitFor(() => expect(onSettled).toHaveBeenCalledTimes(1))
  fireEvent(document, new Event('visibilitychange'))
  expect(onSettled).toHaveBeenCalledTimes(1)
})

it.each([
  'javascript:alert(1)',
  'https://user:password@checkout.example/pay',
  'alipays://pay',
  '//checkout.example/pay',
])('blocks an unsafe hosted URL: %s', async (unsafe) => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: order('pending', unsafe) },
  })
  const post = vi.spyOn(api, 'post')
  mount(
    <AdtyPaymentDialogs
      payment={{
        kind: 'hosted',
        method: 'stripe',
        amount: 10,
        tradeNo: 'HOST1',
      }}
      onClose={() => {}}
      onSettled={() => {}}
    />
  )
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Continue payment' })
  ).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled()
})

it('resumes an existing hosted order and only closes after cancellation succeeds', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: order() },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValueOnce({ data: { success: true, data: null } })
  const onClose = vi.fn()
  mount(
    <AdtyPaymentDialogs
      payment={{
        kind: 'hosted',
        method: 'stripe',
        amount: 10,
        tradeNo: 'HOST1',
      }}
      onClose={onClose}
      onSettled={() => {}}
    />
  )
  await screen.findByRole('link', { name: 'Continue payment' })
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Cancel order' }))
  expect(await screen.findByText('offline')).toBeVisible()
  expect(onClose).not.toHaveBeenCalled()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Cancel order' }))
  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenLastCalledWith('/api/adty/topup/orders/HOST1/cancel')
})

it.each(['info', 'history'])(
  'shows a retry for failed wallet %s without pretending it is empty',
  async (section) => {
    let failed = true
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      if (
        failed &&
        path.endsWith(section === 'info' ? '/topup/info' : '/topup/self')
      ) {
        throw new Error('offline')
      }
      let data: unknown = { id: 1, quota: 0, used_quota: 0, request_count: 0 }
      if (path.endsWith('/topup/info')) data = info
      else if (path.endsWith('/topup/self')) data = { items: [], total: 0 }
      return { data: { success: true, data } }
    })
    mount(<Wallet />)
    const retry = await screen.findByRole('button', { name: 'Retry' })
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
  }
)

it('shows the new hosted order in the pending banner after closing its checkout dialog', async () => {
  let created = false
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    let data: unknown = { id: 1, quota: 0, used_quota: 0, request_count: 0 }
    if (path.endsWith('/topup/info')) {
      data = {
        ...info,
        min_topup: 1,
        methods: [
          { id: 'stripe', kind: 'hosted', label: 'Stripe', available: true },
        ],
        pending: created ? order() : null,
      }
    } else if (path.endsWith('/topup/self')) data = { items: [], total: 0 }
    else if (path.includes('/topup/orders/')) data = order()
    return { data: { success: true, data } }
  })
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (String(url).endsWith('/hosted/pay')) {
      created = true
      return {
        data: {
          message: 'success',
          data: {
            order_id: 'HOST1',
            checkout_url: 'https://checkout.example/pay',
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: {
          available: true,
          quote: {
            face_usd: '10',
            bonus_usd: '0',
            promo_bonus_usd: '0',
            pay_amount: '9',
            pay_currency: 'EUR',
          },
        },
      },
    }
  })
  const user = userEvent.setup()
  mount(<Wallet />)
  await user.type(await screen.findByLabelText('Custom amount (USD)'), '10')
  await user.click(screen.getByRole('button', { name: 'Stripe' }))
  const pay = screen.getByRole('button', { name: 'Pay now' })
  await waitFor(() => expect(pay).toBeEnabled())
  await user.click(pay)
  await screen.findByRole('link', { name: 'Continue payment' })
  await user.click(screen.getByRole('button', { name: 'Close' }))
  expect(
    await screen.findByRole('button', { name: 'Continue payment' })
  ).toBeVisible()
})
