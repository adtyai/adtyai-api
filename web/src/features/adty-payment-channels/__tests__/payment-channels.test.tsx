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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import type { EpayChannel } from '../api'
import { PaymentChannels } from '../index'

const api = vi.hoisted(() => ({
  getEpayChannels: vi.fn(),
  saveEpayChannel: vi.fn(),
  deleteEpayChannel: vi.fn(),
  setEpayChannelStatus: vi.fn(),
  testEpayChannel: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))
vi.mock('@/components/layout', () => {
  const Layout = (props: { children: ReactNode }) => <div>{props.children}</div>
  Layout.Title = (props: { children: ReactNode }) => <h1>{props.children}</h1>
  Layout.Content = (props: { children: ReactNode }) => (
    <div>{props.children}</div>
  )
  return { SectionPageLayout: Layout }
})

const channel: EpayChannel = {
  id: 3,
  name: 'ZPay A',
  gateway_url: 'https://z-pay.cn',
  merchant_id: '1001',
  priority: 5,
  weight: 1,
  order_mode: 'api',
  probe_mode: 'off',
  status: 'enabled',
  status_reason: '',
  unpaid_streak: 2,
  unpaid_threshold: 0,
  has_key: true,
  today_cny: { alipay: '730.00' },
  methods: [
    {
      method: 'alipay',
      type_code: 'alipay',
      min_cny: '0',
      max_cny: '0',
      daily_limit_cny: '0',
      return_type: 'qrcode',
      enabled: true,
    },
  ],
}

function renderPage(role: number, permissions?: Record<string, boolean>) {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'op',
    role,
    permissions: permissions
      ? { admin_permissions: { adty_payment: permissions } }
      : undefined,
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <PaymentChannels />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  api.getEpayChannels.mockResolvedValue([channel])
})
afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.clearAllMocks()
})

describe('PaymentChannels', () => {
  it('lists channels with methods, status, unpaid streak and today', async () => {
    renderPage(10, { read: true })
    expect(await screen.findByText('ZPay A')).toBeVisible()
    expect(screen.getByText('Alipay (qrcode)')).toBeVisible()
    expect(screen.getByText('Alipay ¥730.00')).toBeVisible()
  })

  it('hides add and edit from admins who are not root', async () => {
    renderPage(10, { read: true, operate: true })
    await screen.findByText('ZPay A')
    expect(
      screen.queryByRole('button', { name: 'Add payment channel' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Test order' })).toBeVisible()
  })

  it('opens the drawer for root with an empty key field and default type codes', async () => {
    const user = userEvent.setup()
    renderPage(100)
    await user.click(await screen.findByRole('button', { name: 'Edit' }))
    const drawer = await screen.findByRole('dialog')
    expect(within(drawer).getByLabelText('Merchant key')).toHaveValue('')
    expect(within(drawer).getByLabelText('Type code')).toHaveValue('alipay')
  })

  it('requires at least one method before saving a new channel', async () => {
    const user = userEvent.setup()
    renderPage(100)
    await user.click(
      await screen.findByRole('button', { name: 'Add payment channel' })
    )
    const drawer = await screen.findByRole('dialog')
    expect(
      within(drawer).getByText('Enable at least one payment method')
    ).toBeVisible()
    expect(within(drawer).getByRole('button', { name: 'Save' })).toBeDisabled()
  })
})
