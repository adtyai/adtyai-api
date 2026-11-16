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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SiteFundPanel } from '../components/site-fund-panel'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

/**
 * ISS-R4-0016：`wallets.isSuccess` 为真但响应体不是数组时，面板原来直接 `.map`
 * 会抛 TypeError 让整块资金页白屏。列表形状不可信，就必须兜底。
 */
it.each([
  ['null', null],
  ['an object', { items: [] }],
  ['a string', 'unexpected'],
])(
  'survives a wallets payload that is %s',
  async (_label, walletsPayload) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      if (path.endsWith('/wallets')) {
        return { data: { success: true, data: walletsPayload } }
      }
      if (path.endsWith('/commission-margin')) {
        return { data: { success: true, data: { site_id: 1, allowed: false } } }
      }
      if (path.endsWith('/fund-anomalies')) {
        return { data: { success: true, data: { kinds: [] } } }
      }
      return { data: { success: true, data: { items: [] } } }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <SiteFundPanel siteId={1} />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Wallets')).toBeInTheDocument()
    // 「Issue margin code」在处置列表之后渲染：若处置列表抛错，整棵树会挂掉，
    // 这一段就不存在了。只断言 'No data' 不够——它可能来自上面的钱包区块。
    expect(
      await screen.findAllByText('Issue margin code')
    ).not.toHaveLength(0)
    client.clear()
  }
)

it('still lists the wallets when the payload is an array', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    if (path.endsWith('/wallets')) {
      return {
        data: {
          success: true,
          data: [{ id: 3, kind: 'margin', balance: '12.50' }],
        },
      }
    }
    if (path.endsWith('/commission-margin')) {
      return { data: { success: true, data: { site_id: 1, allowed: false } } }
    }
    if (path.endsWith('/fund-anomalies')) {
      return { data: { success: true, data: { kinds: [] } } }
    }
    return { data: { success: true, data: { items: [] } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteFundPanel siteId={1} />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText(/^Margin wallet: 12\.50 USD$/)
  ).toBeInTheDocument()
  client.clear()
})

it('pages the wallet ledger and filters it by wallet', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    if (path.endsWith('/wallet-ledgers')) {
      return {
        data: {
          success: true,
          data: {
            total: 1,
            page: 1,
            page_size: 20,
            items: [
              {
                id: 9,
                site_id: 1,
                wallet_id: 3,
                wallet_kind: 'earnings',
                delta: '-5.5',
                hold_delta: '0',
                balance_before: '20',
                balance_after: '14.5',
                type: 'earning_withdraw_freeze',
                ref_type: 'site_withdraw',
                ref_id: 'wd-1',
                reason: '',
                operator_id: 7,
                allow_overdraft: false,
                created_at: 1,
              },
            ],
          },
        },
      }
    }
    if (path.endsWith('/wallets')) return { data: { success: true, data: [] } }
    if (path.endsWith('/commission-margin')) {
      return { data: { success: true, data: { site_id: 1, allowed: false } } }
    }
    return { data: { success: true, data: { items: [] } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteFundPanel siteId={1} />
    </QueryClientProvider>
  )
  const table = await screen.findByRole('table', { name: 'Wallet ledger' })
  expect(table).toHaveTextContent('Earnings withdrawal')
  expect(table).toHaveTextContent('-5.5 USD')
  expect(table).toHaveTextContent('20 → 14.5')
  expect(table).toHaveTextContent('#7')
  expect(get).toHaveBeenCalledWith('/api/adty/admin/sites/1/wallet-ledgers', {
    params: { kind: undefined, p: 1, page_size: 20 },
  })
  await userEvent.setup().selectOptions(
    screen.getByRole('combobox', { name: 'Wallet' }),
    'operating'
  )
  expect(get).toHaveBeenCalledWith('/api/adty/admin/sites/1/wallet-ledgers', {
    params: { kind: 'operating', p: 1, page_size: 20 },
  })
  client.clear()
})

/**
 * 异常处置列表和钱包列表一样，是「成功但形状不可信」的高危点：
 * 崩了站长就看不出哪笔资金异常还没处置完。这次连 dispositions 一起兜底。
 */
it.each([
  ['null', null],
  ['an object', { items: [] }],
  ['a string', 'unexpected'],
])(
  'survives a dispositions payload that is %s',
  async (_label, dispositionsPayload) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      if (path.endsWith('/wallets')) {
        return { data: { success: true, data: [] } }
      }
      if (path.endsWith('/dispositions')) {
        return { data: { success: true, data: dispositionsPayload } }
      }
      if (path.endsWith('/commission-margin')) {
        return { data: { success: true, data: null } }
      }
      if (path.endsWith('/fund-anomalies')) {
        return { data: { success: true, data: { kinds: [] } } }
      }
      return { data: { success: true, data: { items: [] } } }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <SiteFundPanel siteId={1} />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Wallets')).toBeInTheDocument()
    expect(await screen.findAllByText('No data')).not.toHaveLength(0)
    client.clear()
  }
)
