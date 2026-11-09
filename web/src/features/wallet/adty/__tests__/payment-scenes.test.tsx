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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { payWithBEpusdt, payWithEpay, payWithHosted } from '../api-pay'
import { AdtyRechargeCard } from '../components/adty-recharge-card'
import type { AdtyTopupInfo } from '../types'

const originalAdapter = api.defaults.adapter
let client: QueryClient
let requests: { path: string; body: Record<string, unknown> }[]
const usdt = { id: 'usdt.trc20', kind: 'usdt' as const, available: true }
const alipay = { id: 'alipay', kind: 'epay' as const, available: true }
const info = {
  tiers: [{ amount: 100, rate: 0 }],
  bonus_rates: {},
  min_topup: 1,
  price_cny_per_usd: 7,
  usdt_order_minutes: 15,
  epay_order_minutes: 10,
  pending: null,
  compliance_confirmed: true,
  methods: [usdt, alipay],
  scenes: [
    {
      key: 'wallet_topup',
      name: 'Wallet top-up',
      min_usd: '0',
      max_usd: '0',
      methods: [usdt],
    },
    {
      key: 'limited',
      name: 'Local checkout',
      min_usd: '50',
      max_usd: '100',
      methods: [alipay],
    },
  ],
} as AdtyTopupInfo

beforeEach(() => {
  requests = []
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  api.defaults.adapter = (async (config) => {
    const path = config.url ?? ''
    const body = config.data ? JSON.parse(config.data) : {}
    requests.push({ path, body })
    let data: unknown = null
    if (path.endsWith('/quote')) {
      data = {
        available: true,
        quote: {
          face_usd: '100',
          bonus_rate: '0',
          bonus_usd: '0',
          credit_quota: 50000000,
          pay_amount: body.method === 'alipay' ? '700' : '100',
          pay_currency: body.method === 'alipay' ? 'CNY' : 'USDT',
          price_cny_per_usd: '7',
          topup_group_ratio: '1',
          promo_code_id: 0,
          promo_bonus_usd: '0',
        },
      }
    }
    if (path.endsWith('/hosted/pay')) {
      data = { order_id: 'SCENE1', pay_link: 'https://checkout.example/scene' }
    }
    return {
      data: {
        success: true,
        message: path.endsWith('/hosted/pay') ? 'success' : '',
        data,
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }) as AxiosAdapter
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
})

it('switching scenes clears the selected method and sends the new scene with the quote and payment', async () => {
  const user = userEvent.setup()
  const onPay = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <AdtyRechargeCard
        info={info}
        loading={false}
        paying={false}
        onPay={onPay}
      />
    </QueryClientProvider>
  )
  expect(
    screen.queryByRole('button', { name: 'Alipay' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /\$100/ }))
  await user.click(screen.getByRole('button', { name: /TRC20/ }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeEnabled()
  )
  await user.selectOptions(screen.getByLabelText('Payment scene'), 'limited')
  expect(
    screen.queryByRole('button', { name: /TRC20/ })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Pay now' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Alipay' }))
  await waitFor(() =>
    expect(requests.at(-1)?.body.payment_scene).toBe('limited')
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeEnabled()
  )
  await user.click(screen.getByRole('button', { name: 'Pay now' }))
  expect(onPay).toHaveBeenCalledWith(100, alipay, '', 'limited')
})

it('an empty authorized scene list keeps payment disabled even if old methods are cached', () => {
  render(
    <QueryClientProvider client={client}>
      <AdtyRechargeCard
        info={{ ...info, scenes: [] }}
        loading={false}
        paying={false}
        onPay={() => {}}
      />
    </QueryClientProvider>
  )
  expect(
    screen.queryByRole('button', { name: /TRC20/ })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Alipay' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Pay now' })).toBeDisabled()
})

it('all wallet payment clients carry the selected scene to their native or unified HTTP endpoint', async () => {
  await payWithBEpusdt(100, 'usdt.trc20', '', 'limited')
  await payWithEpay(100, 'alipay', 'pc', [], '', 'limited')
  await payWithHosted(100, 'stripe', '', 'limited')
  expect(
    requests.map((request) => [request.path, request.body.payment_scene])
  ).toEqual([
    ['/api/user/bepusdt/pay', 'limited'],
    ['/api/adty/epay/pay', 'limited'],
    ['/api/adty/topup/hosted/pay', 'limited'],
  ])
})
