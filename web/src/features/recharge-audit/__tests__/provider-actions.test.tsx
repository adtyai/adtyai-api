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
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { RechargeAudit } from '../index'

const providers = [
  { provider: 'bepusdt', label: 'BEpusdt', query: false },
  { provider: 'adty_epay', label: 'Epay channels', query: true },
  { provider: 'stripe', label: 'Stripe', query: true },
  { provider: 'creem', label: 'Creem', query: true },
  { provider: 'waffo', label: 'Waffo', query: true },
  { provider: 'waffo_pancake', label: 'Waffo Pancake', query: true },
]
const clients: QueryClient[] = []
const orders = providers.flatMap((provider, index) =>
  ['pending', 'success'].map((status, position) => ({
    id: index * 2 + position,
    user_id: 7,
    username: 'buyer',
    site_id: 7 as number | null,
    site_name: 'North Studio',
    order_type: 'topup',
    trade_no: `trade-${provider.provider}-${status}`,
    payment_provider: provider.provider,
    payment_method: provider.provider,
    status,
    amount: 2,
    money: 2,
    face_usd: '2',
    bonus_usd: '0',
    pay_amount: '2',
    pay_currency: 'USD',
    paid_amount: status === 'success' ? '2' : '0',
    paid_currency: 'USD',
    has_ext: true,
    create_time: 1,
    complete_time: status === 'success' ? 2 : 0,
    actions: {
      complete: status === 'pending',
      refund: status === 'success',
      query: provider.query && status === 'pending',
    },
  }))
)
orders[0] = { ...orders[0], site_id: 0, site_name: 'Main Studio' }
orders[1] = { ...orders[1], site_id: null, site_name: '' }

function setPermissions(write: boolean) {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'finance',
    role: 10,
    permissions: {
      admin_permissions: {
        adty_finance: { read: true, manual_credit: write, refund: write },
      },
    },
  })
}

function renderAudit(write = true, ownerMode = false) {
  setPermissions(write)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <RechargeAudit ownerMode={ownerMode} />
    </QueryClientProvider>
  )
}

function emptyTrace(ownerMode = false) {
  return {
    data: {
      success: true,
      data: {
        order: orders.find((item) => item.trade_no === 'trade-stripe-success'),
        refund: {
          refunded_at: 0,
          refund_quota: 0,
          refund_amount: '0',
          currency: 'USD',
        },
        sections: ownerMode
          ? ['payments', 'wallet']
          : ['payments', 'wallet', 'commissions'],
        section: 'payments',
        page: 1,
        page_size: 20,
        total: 0,
        items: [],
        promo_bonus_usd: '0',
      },
    },
  }
}

function currentGetMock() {
  const handler = vi.mocked(api.get).getMockImplementation()
  if (!handler) {
    throw new Error('Expected the network fixture to be installed')
  }
  return handler
}

it('uses the owner detail endpoint and hides platform commission sections', async () => {
  const originalGet = currentGetMock()
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/adty/site/orders/trade-stripe-success') {
      return emptyTrace(true)
    }
    return originalGet(url, config)
  })
  renderAudit(false, true)
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'trade-stripe-success' })
  )
  const dialog = within(
    await screen.findByRole('dialog', { name: 'Order details' })
  )
  expect(await dialog.findByText('No linked records')).toBeVisible()
  expect(
    dialog.queryByRole('option', { name: 'Commission records' })
  ).not.toBeInTheDocument()
  expect(
    dialog.queryByRole('option', { name: 'Commission wallet movements' })
  ).not.toBeInTheDocument()
})

it('shows a retry after detail loading fails and displays the recovered empty result', async () => {
  const originalGet = currentGetMock()
  let failed = true
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/admin/finance/recharge-audit/trade-stripe-success') {
      if (failed) {
        return { data: { success: false, message: 'Synthetic detail failure' } }
      }
      return emptyTrace()
    }
    return originalGet(url, config)
  })
  renderAudit(false)
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'trade-stripe-success' })
  )
  const dialog = within(
    await screen.findByRole('dialog', { name: 'Order details' })
  )
  const retry = await dialog.findByRole('button', { name: 'Retry' })
  failed = false
  await user.click(retry)
  expect(await dialog.findByText('No linked records')).toBeVisible()
})

it('closes details immediately when the account changes and ignores its late response', async () => {
  const originalGet = currentGetMock()
  let finish: ((value: ReturnType<typeof emptyTrace>) => void) | undefined
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/admin/finance/recharge-audit/trade-stripe-success') {
      return await new Promise<ReturnType<typeof emptyTrace>>((resolve) => {
        finish = resolve
      })
    }
    return originalGet(url, config)
  })
  renderAudit(false)
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'trade-stripe-success' })
  )
  expect(
    await screen.findByRole('dialog', { name: 'Order details' })
  ).toBeVisible()
  await waitFor(() => expect(finish).toBeTypeOf('function'))
  act(() =>
    useAuthStore
      .getState()
      .auth.setUser({ id: 3, username: 'other-finance', role: 10 })
  )
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await act(async () => finish?.(emptyTrace()))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  act(() => setPermissions(false))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const ownerMode = url.startsWith('/api/adty/site/orders')
    if (
      url === '/api/user/admin/finance/recharge-audit/summary' ||
      url === '/api/adty/site/orders/summary'
    ) {
      return {
        data: {
          success: true,
          data: {
            orders: 12,
            credited_usd: '12',
            bonus_usd: '0',
            currency: 'USD',
            provider_option: providers.map((provider) => provider.provider),
            site_options: ownerMode
              ? [{ site_id: 7, site_name: 'North Studio' }]
              : [
                  { site_id: 0, site_name: 'Main Studio' },
                  { site_id: 7, site_name: 'North Studio' },
                ],
            provider_capabilities: providers.map((provider) => ({
              ...provider,
              complete: true,
              refund: true,
              refund_mode: 'offline',
            })),
          },
        },
      }
    }
    if (url === '/api/user/admin/finance/payment-orphans') {
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    if (
      url !== '/api/user/admin/finance/recharge-audit' &&
      url !== '/api/adty/site/orders'
    ) {
      throw new Error(`Unexpected GET ${url}`)
    }
    const params = config?.params as
      | { provider?: string; site_id?: number }
      | undefined
    const items = orders.filter((order) => {
      if (ownerMode && order.payment_provider !== 'stripe') {
        return false
      }
      if (params?.provider && order.payment_provider !== params.provider) {
        return false
      }
      return params?.site_id === undefined || order.site_id === params.site_id
    })
    return { data: { success: true, data: { items, total: items.length } } }
  })
})

it('shows order site snapshots, explicit unknown ownership, type and completion time', async () => {
  renderAudit(false)
  await screen.findByText('trade-bepusdt-pending')
  expect(
    screen.getByRole('heading', { name: 'Order management' })
  ).toBeVisible()
  const table = within(screen.getByRole('table', { name: 'Order management' }))
  expect(table.getByRole('columnheader', { name: 'Site' })).toBeVisible()
  expect(table.getByRole('columnheader', { name: 'Site ID' })).toBeVisible()
  expect(table.getByRole('columnheader', { name: 'Order type' })).toBeVisible()
  expect(
    table.getByRole('columnheader', { name: 'Completed at' })
  ).toBeVisible()
  const main = within(
    screen.getByRole('row', { name: /trade-bepusdt-pending/ })
  )
  expect(main.getByText('Main Studio')).toBeVisible()
  expect(main.getByRole('cell', { name: '0' })).toBeVisible()
  expect(main.getByText('Top-up')).toBeVisible()
  const legacy = within(
    screen.getByRole('row', { name: /trade-bepusdt-success/ })
  )
  expect(legacy.getByText('Unknown site')).toBeVisible()
  expect(screen.getAllByText('North Studio').length).toBeGreaterThan(0)
})

it('opens order details from the order number and paginates the selected trace section', async () => {
  const originalGet = currentGetMock()
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/admin/finance/recharge-audit/trade-stripe-success') {
      const params = config?.params as { section: string; p: number }
      return {
        data: {
          success: true,
          data: {
            order: orders.find(
              (item) => item.trade_no === 'trade-stripe-success'
            ),
            refund: {
              refunded_at: 0,
              refund_quota: 0,
              refund_amount: '0',
              currency: 'USD',
            },
            section: params.section,
            sections: [
              'payments',
              'wallet',
              'bonuses',
              'commissions',
              'commission_movements',
              'site_wallet',
              'exceptions',
              'reviews',
            ],
            page: params.p,
            page_size: 20,
            total: params.section === 'wallet' ? 21 : 1,
            items: [
              {
                id: params.p,
                kind: params.section === 'wallet' ? 'topup' : 'payment',
                source: 'callback',
                status: 'credited',
                amount: '2',
                currency: 'USD',
                created_at: 2,
                reference: `evidence-page-${params.p}`,
              },
            ],
          },
        },
      }
    }
    return originalGet(url, config)
  })
  renderAudit(false)
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'trade-stripe-success' })
  )
  const dialog = within(
    await screen.findByRole('dialog', { name: 'Order details' })
  )
  expect(await dialog.findByText('evidence-page-1')).toBeVisible()
  expect(
    dialog.getByText(
      'Earlier payment attempts may not have been recorded. The order snapshot is not a complete callback history.'
    )
  ).toBeVisible()
  await user.selectOptions(
    dialog.getByRole('combobox', { name: 'Trace records' }),
    'wallet'
  )
  await user.click(await dialog.findByRole('button', { name: 'Next' }))
  expect(await dialog.findByText('evidence-page-2')).toBeVisible()
  expect(dialog.getByRole('button', { name: 'Next' })).toBeDisabled()
})

it('sends the main site zero and order type filters and displays only matching orders', async () => {
  const user = userEvent.setup()
  renderAudit(false)
  await screen.findByText('trade-creem-pending')
  const site = screen.getByRole('combobox', { name: 'Site' })
  await user.selectOptions(site, '0')
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Order type' }),
    'topup'
  )
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/user/admin/finance/recharge-audit',
      expect.objectContaining({
        params: expect.objectContaining({
          site_id: 0,
          order_type: 'topup',
          p: 1,
        }),
      })
    )
  )
  expect(await screen.findByText('trade-bepusdt-pending')).toBeVisible()
  await waitFor(() =>
    expect(screen.queryByText('trade-creem-pending')).not.toBeInTheDocument()
  )
  await user.selectOptions(site, '')
  expect(await screen.findByText('trade-creem-pending')).toBeVisible()
})

it('uses site-scoped reads and hides global anomalies and all financial actions in owner mode', async () => {
  renderAudit(true, true)
  expect(await screen.findByText('trade-stripe-success')).toBeVisible()
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/adty/site/orders',
      expect.anything()
    )
  )
  expect(screen.queryByText('trade-creem-pending')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('combobox', { name: 'Site' })
  ).not.toBeInTheDocument()
  for (const action of ['Complete', 'Query payment', 'Refund']) {
    expect(
      screen.queryByRole('button', { name: action })
    ).not.toBeInTheDocument()
  }
  expect(
    vi
      .mocked(api.get)
      .mock.calls.some(
        ([url]) => url === '/api/user/admin/finance/payment-orphans'
      )
  ).toBe(false)
  expect(
    vi
      .mocked(api.get)
      .mock.calls.some(([url]) => url.startsWith('/api/user/admin/finance/'))
  ).toBe(false)
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

it('offers all six registered providers and sends the selected provider to the API', async () => {
  const user = userEvent.setup()
  renderAudit()
  const filter = await screen.findByRole('combobox', { name: 'Provider' })
  await screen.findByRole('option', { name: 'Waffo Pancake' })
  for (const provider of providers) {
    expect(
      within(filter).getByRole('option', { name: provider.label })
    ).toHaveValue(provider.provider)
  }
  await user.selectOptions(filter, 'stripe')
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/user/admin/finance/recharge-audit',
      expect.objectContaining({
        params: expect.objectContaining({ provider: 'stripe', p: 1 }),
      })
    )
  )
  expect(await screen.findByText('trade-stripe-pending')).toBeVisible()
  await waitFor(() =>
    expect(screen.queryByText('trade-creem-pending')).not.toBeInTheDocument()
  )
})

it('shows Hosted completion, query and offline refund actions while respecting per-order capabilities', async () => {
  renderAudit()
  await screen.findByText('trade-creem-pending')
  for (const provider of ['stripe', 'creem', 'waffo', 'waffo_pancake']) {
    const pending = within(
      screen.getByRole('row', { name: new RegExp(`trade-${provider}-pending`) })
    )
    expect(pending.getByRole('button', { name: 'Complete' })).toBeEnabled()
    expect(pending.getByRole('button', { name: 'Query payment' })).toBeEnabled()
    expect(
      pending.queryByRole('button', { name: 'Refund' })
    ).not.toBeInTheDocument()
    const paid = within(
      screen.getByRole('row', { name: new RegExp(`trade-${provider}-success`) })
    )
    expect(paid.getByRole('button', { name: 'Refund' })).toBeEnabled()
    expect(
      paid.queryByRole('button', { name: 'Complete' })
    ).not.toBeInTheDocument()
    expect(
      paid.queryByRole('button', { name: 'Query payment' })
    ).not.toBeInTheDocument()
  }
  const bep = within(screen.getByRole('row', { name: /trade-bepusdt-pending/ }))
  expect(
    bep.queryByRole('button', { name: 'Query payment' })
  ).not.toBeInTheDocument()
})

it('hides all write actions from a read-only finance user', async () => {
  renderAudit(false)
  await screen.findByText('trade-creem-pending')
  for (const name of ['Complete', 'Refund', 'Query payment']) {
    expect(screen.queryByRole('button', { name })).not.toBeInTheDocument()
  }
})

it('keeps a failed provider query open for retry and distinguishes an unconfirmed payment', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'post')
    .mockResolvedValueOnce({
      data: { success: false, message: 'Payment gateway is unavailable' },
    })
    .mockResolvedValueOnce({
      data: { success: true, data: { status: 'pending' } },
    })
  renderAudit()
  const row = await screen.findByRole('row', { name: /trade-creem-pending/ })
  await user.click(within(row).getByRole('button', { name: 'Query payment' }))
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Query payment',
  })
  await user.click(
    within(dialog).getByRole('button', { name: 'Query payment' })
  )
  expect(await within(dialog).findByRole('alert')).toHaveTextContent(
    'Payment gateway is unavailable'
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Query payment' })
  )
  expect(
    await within(dialog).findByText('No confirmed payment found')
  ).toBeVisible()
  expect(api.post).toHaveBeenCalledWith(
    '/api/adty/admin/topup/trade-creem-pending/query'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(within(row).getByRole('button', { name: 'Complete' })).toBeEnabled()
})

it('removes the query dialog when finance write permission is revoked', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post')
  renderAudit()
  const row = await screen.findByRole('row', { name: /trade-creem-pending/ })
  await user.click(within(row).getByRole('button', { name: 'Query payment' }))
  expect(
    await screen.findByRole('alertdialog', { name: 'Query payment' })
  ).toBeVisible()
  act(() => setPermissions(false))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(post).not.toHaveBeenCalled()
})
