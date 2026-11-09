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
    actions: {
      complete: status === 'pending',
      refund: status === 'success',
      query: provider.query && status === 'pending',
    },
  }))
)

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

function renderAudit(write = true) {
  setPermissions(write)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <RechargeAudit />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/user/admin/finance/recharge-audit/summary') {
      return {
        data: {
          success: true,
          data: {
            orders: 12,
            credited_usd: '12',
            bonus_usd: '0',
            currency: 'USD',
            provider_option: providers.map((provider) => provider.provider),
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
    if (url !== '/api/user/admin/finance/recharge-audit') {
      throw new Error(`Unexpected GET ${url}`)
    }
    const provider = (config?.params as { provider?: string } | undefined)
      ?.provider
    const items = provider
      ? orders.filter((order) => order.payment_provider === provider)
      : orders
    return { data: { success: true, data: { items, total: items.length } } }
  })
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
