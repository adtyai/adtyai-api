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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import type { RechargeAuditOrder } from '../api'
import { RechargeAudit } from '../index'

const api = vi.hoisted(() => ({
  getRechargeAudit: vi.fn(),
  getRechargeAuditSummary: vi.fn(),
  getPaymentOrphans: vi.fn(),
  previewTopupRefund: vi.fn(),
  refundTopup: vi.fn(),
  completeTopupManually: vi.fn(),
  creditPaymentOrphan: vi.fn(),
  resolvePaymentOrphan: vi.fn(),
}))
vi.mock('../api', () => api)
vi.mock('@/components/layout', () => {
  const Layout = (props: { children: ReactNode }) => <div>{props.children}</div>
  Layout.Title = (props: { children: ReactNode }) => <h1>{props.children}</h1>
  Layout.Content = (props: { children: ReactNode }) => (
    <div>{props.children}</div>
  )
  return { SectionPageLayout: Layout }
})

const order: RechargeAuditOrder = {
  id: 1,
  user_id: 7,
  username: 'buyer',
  trade_no: 'EPY7001',
  payment_method: 'alipay',
  payment_provider: 'adty_epay',
  status: 'success',
  create_time: 1,
  complete_time: 2,
  amount: 100,
  money: 730,
  has_ext: true,
  face_usd: '100',
  bonus_usd: '2.5',
  credit_quota: 51250000,
  pay_amount: '730',
  pay_currency: 'CNY',
  paid_amount: '730',
  paid_currency: 'CNY',
  paid_tx_hash: '2026103100001',
  paid_source: 'callback',
  channel_id: 3,
  channel_name: 'ZPay A',
  anomaly: '',
  actions: { complete: false, refund: true, query: false },
}

function renderPage(permissions: Record<string, boolean>) {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'finance',
    role: 10,
    permissions: { admin_permissions: { adty_finance: permissions } },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RechargeAudit />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  api.previewTopupRefund.mockResolvedValue({
    deducted_quota: 50000000,
    bonus_quota: 1250000,
    refund_quota: 50000000,
    refund_amount: '100',
    currency: 'USDT',
  })
  api.getRechargeAudit.mockResolvedValue({
    page: 1,
    page_size: 20,
    total: 1,
    items: [order],
  })
  api.getRechargeAuditSummary.mockResolvedValue({
    orders: 1,
    credited_usd: '100',
    bonus_usd: '2.5',
    currency: 'USD',
    status_options: [],
    provider_option: [],
  })
  api.getPaymentOrphans.mockResolvedValue({
    page: 1,
    page_size: 20,
    total: 0,
    items: [],
  })
})

afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.clearAllMocks()
})

describe('RechargeAudit', () => {
  it('shows face value, bonus, payment and channel of each order', async () => {
    renderPage({ read: true })
    expect(await screen.findByText('EPY7001')).toBeVisible()
    expect(screen.getAllByText('$100.00').length).toBeGreaterThan(0)
    expect(screen.getAllByText('$2.50').length).toBeGreaterThan(0)
    expect(screen.getAllByText('730 CNY')).toHaveLength(2)
    expect(screen.getByText('ZPay A')).toBeVisible()
  })

  it('hides refund and completion buttons without those permissions', async () => {
    renderPage({ read: true })
    await screen.findByText('EPY7001')
    expect(
      screen.queryByRole('button', { name: 'Refund' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Complete' })
    ).not.toBeInTheDocument()
  })

  it('explains that the bonus is not refunded before confirming', async () => {
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    const dialog = await screen.findByRole('dialog', { name: 'Refund order' })
    expect(within(dialog).getByText(/The bonus is not refunded/)).toBeVisible()
  })

  it('shows the quota taken back and the amount to return after refunding', async () => {
    api.previewTopupRefund.mockResolvedValue({
      deducted_quota: 40000000,
      bonus_quota: 1250000,
      refund_quota: 38750000,
      refund_amount: '565.75',
      currency: 'CNY',
    })
    api.refundTopup.mockResolvedValue({
      deducted_quota: 40000000,
      bonus_quota: 1250000,
      refund_quota: 38750000,
      refund_amount: '565.75',
      currency: 'CNY',
    })
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    await user.type(screen.getByLabelText('Reason (required)'), 'chargeback')
    const dialog = await screen.findByRole('dialog', { name: 'Refund order' })
    await screen.findByText(/Refund 565\.75 CNY/)
    await user.click(within(dialog).getByRole('button', { name: 'Refund' }))
    const done = await screen.findByRole('dialog', { name: 'Order refunded' })
    expect(within(done).getByText(/Refund 565\.75 CNY/)).toBeVisible()
    expect(api.refundTopup.mock.calls[0][3]).toBe(38750000)
    expect(
      within(done).queryByText(/Nothing to refund/)
    ).not.toBeInTheDocument()
  })

  it('says nothing is refunded when the credit left is only bonus', async () => {
    api.previewTopupRefund.mockResolvedValue({
      deducted_quota: 0,
      bonus_quota: 1250000,
      refund_quota: 0,
      refund_amount: '0',
      currency: 'USDT',
    })
    api.refundTopup.mockResolvedValue({
      deducted_quota: 500000,
      bonus_quota: 1250000,
      refund_quota: 0,
      refund_amount: '0',
      currency: 'USDT',
    })
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    await user.type(screen.getByLabelText('Reason (required)'), 'chargeback')
    const dialog = await screen.findByRole('dialog', { name: 'Refund order' })
    await screen.findByText(/Refund 0 USDT/)
    await user.click(within(dialog).getByRole('button', { name: 'Refund' }))
    const done = await screen.findByRole('dialog', { name: 'Order refunded' })
    expect(within(done).getByText(/Refund 0 USDT/)).toBeVisible()
    expect(within(done).getByText(/Nothing to refund/)).toBeVisible()
  })

  it('requires a reason before refunding', async () => {
    api.refundTopup.mockResolvedValue({
      deducted_quota: 0,
      bonus_quota: 0,
      refund_quota: 0,
      refund_amount: '0',
      currency: 'CNY',
    })
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    const dialog = await screen.findByRole('dialog', { name: 'Refund order' })
    const confirm = within(dialog).getByRole('button', { name: 'Refund' })
    expect(confirm).toBeDisabled()
    await user.type(screen.getByLabelText('Reason (required)'), 'chargeback')
    await screen.findByText(/Refund 100 USDT/)
    expect(confirm).toBeEnabled()
    await user.click(confirm)
    await waitFor(() => expect(api.refundTopup).toHaveBeenCalled())
    expect(api.refundTopup.mock.calls[0][0]).toBe('EPY7001')
    expect(api.refundTopup.mock.calls[0][1]).toBe('chargeback')
  })
})
