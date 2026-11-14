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
  listTopupManualRefunds: vi.fn(),
  runTopupManualRefundStep: vi.fn(),
  completeTopupManually: vi.fn(),
  creditPaymentOrphan: vi.fn(),
  resolvePaymentOrphan: vi.fn(),
}))
vi.mock('../api', () => api)
vi.mock('@/features/auth/secure-verification/hooks/use-secure-verification', () => ({
  useSecureVerification: () => ({
    requestVerification: vi.fn().mockResolvedValue({ proof_token: 'proof' }),
    dialogProps: { state: { phase: 'idle' } },
    isActive: false,
    cancel: vi.fn(),
  }),
}))
vi.mock(
  '@/features/auth/secure-verification/components/secure-verification-dialog',
  () => ({
    SecureVerificationDialog: () => null,
  })
)
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
  site_id: 7,
  site_name: 'North Studio',
  order_type: 'topup',
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
    facts: {
      trade_no: 'EPY7001',
      site_id: 7,
      user_id: 7,
      face_usd: '100',
      pay_currency: 'CNY',
      paid_amount: '730',
      paid_currency: 'CNY',
      order_rate: '7.3',
      pending_quota: 0,
      refund_occupied_quota: 0,
      refund_completed_usd: '0',
      refund_in_progress_usd: '0',
      refund_unknown_usd: '0',
      refund_remaining_usd: '100',
      bonus_usd: '2.5',
      bonus_used_quota: 0,
      bonus_remaining_quota: 1250000,
      cumulative_refund_usd: '0',
    },
  })
  api.listTopupManualRefunds.mockResolvedValue({
    occupancy: {
      face_quota: 50000000,
      completed_quota: 0,
      in_progress_quota: 0,
      unknown_quota: 0,
      occupied_quota: 0,
      remaining_quota: 50000000,
      face_usd: '100',
      completed_usd: '0',
      in_progress_usd: '0',
      unknown_usd: '0',
      occupied_usd: '0',
      remaining_usd: '100',
    },
    facts: {
      trade_no: 'EPY7001',
      site_id: 7,
      user_id: 7,
      face_usd: '100',
      pay_currency: 'CNY',
      paid_amount: '730',
      paid_currency: 'CNY',
      order_rate: '7.3',
      pending_quota: 0,
      refund_occupied_quota: 0,
      refund_completed_usd: '0',
      refund_in_progress_usd: '0',
      refund_unknown_usd: '0',
      refund_remaining_usd: '100',
      bonus_usd: '2.5',
      bonus_used_quota: 0,
      bonus_remaining_quota: 1250000,
      cumulative_refund_usd: '0',
      manual_refunds: [],
    },
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

  it('explains the offline stepwise refund flow', async () => {
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    const dialog = await screen.findByRole('dialog', { name: 'Manual refund' })
    expect(
      within(dialog).getByText(/record the offline payout, and complete/)
    ).toBeVisible()
    expect(within(dialog).getByText('Completed refund (S)')).toBeVisible()
    expect(within(dialog).getByText('In-progress refund (H)')).toBeVisible()
    expect(within(dialog).getByText('Unknown payout (U)')).toBeVisible()
  })

  it('applies a USD refund request without completing payout', async () => {
    api.refundTopup.mockResolvedValue({
      refund: {
        refund_id: 'refund-1',
        trade_no: 'EPY7001',
        status: 'submitted',
        request_usd: '40',
        request_quota: 20000000,
        reason: 'chargeback',
      },
    })
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    const dialog = await screen.findByRole('dialog', { name: 'Manual refund' })
    await user.type(screen.getByLabelText('Refund amount in USD'), '40')
    await user.type(screen.getByLabelText('Reason (required)'), 'chargeback')
    await user.click(within(dialog).getByRole('button', { name: 'Apply refund' }))
    await waitFor(() => expect(api.refundTopup).toHaveBeenCalled())
    expect(api.refundTopup.mock.calls[0][0]).toBe('EPY7001')
    expect(api.refundTopup.mock.calls[0][1]).toBe('chargeback')
    expect(api.refundTopup.mock.calls[0][4]).toBe('40')
  })

  it('lets an operator record an offline payout on an approved refund', async () => {
    api.listTopupManualRefunds.mockResolvedValue({
      occupancy: {
        face_quota: 50000000,
        completed_quota: 0,
        in_progress_quota: 20000000,
        unknown_quota: 0,
        occupied_quota: 20000000,
        remaining_quota: 30000000,
        face_usd: '100',
        completed_usd: '0',
        in_progress_usd: '40',
        unknown_usd: '0',
        occupied_usd: '40',
        remaining_usd: '60',
      },
      facts: {
        trade_no: 'EPY7001',
        site_id: 7,
        user_id: 7,
        face_usd: '100',
        pay_currency: 'CNY',
        paid_amount: '730',
        paid_currency: 'CNY',
        order_rate: '7.3',
        pending_quota: 0,
        refund_occupied_quota: 20000000,
        refund_completed_usd: '0',
        refund_in_progress_usd: '40',
        refund_unknown_usd: '0',
        refund_remaining_usd: '60',
        bonus_usd: '2.5',
        bonus_used_quota: 0,
        bonus_remaining_quota: 1250000,
        cumulative_refund_usd: '0',
        manual_shortfall_usd: '20',
        manual_refunds: [
          {
            refund_id: 'refund-1',
            trade_no: 'EPY7001',
            status: 'approved',
            request_usd: '40',
            request_quota: 20000000,
            reason: 'chargeback',
            approved_at: 2,
            reclaimed_at: 3,
            shortfall_quota: 10000000,
            shortfall_usd: '20',
            bonus_at: 4,
            commission_at: 5,
          },
        ],
      },
    })
    api.runTopupManualRefundStep.mockResolvedValue({
      refund: { refund_id: 'refund-1', payout_status: 'success' },
    })
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    const dialog = await screen.findByRole('dialog', { name: 'Manual refund' })
    expect(within(dialog).getByText(/Manual-handling shortfall/)).toBeVisible()
    await user.type(screen.getByLabelText('Offline payout currency'), 'CNY')
    await user.type(screen.getByLabelText('Offline payout amount'), '292')
    await user.type(screen.getByLabelText('Offline payout voucher'), 'bank-slip-88')
    await user.click(
      within(dialog).getByRole('button', { name: 'Record offline payout' })
    )
    await waitFor(() => expect(api.runTopupManualRefundStep).toHaveBeenCalled())
    expect(api.runTopupManualRefundStep.mock.calls[0][0]).toBe('EPY7001')
    expect(api.runTopupManualRefundStep.mock.calls[0][1]).toBe('refund-1')
    expect(api.runTopupManualRefundStep.mock.calls[0][2]).toBe('payout')
  })

  it('requires a reason and USD amount before applying', async () => {
    api.refundTopup.mockResolvedValue({
      refund: {
        refund_id: 'refund-1',
        trade_no: 'EPY7001',
        status: 'submitted',
        request_usd: '40',
        request_quota: 20000000,
        reason: 'chargeback',
      },
    })
    const user = userEvent.setup()
    renderPage({ read: true, refund: true })
    await user.click(await screen.findByRole('button', { name: 'Refund' }))
    const dialog = await screen.findByRole('dialog', { name: 'Manual refund' })
    const confirm = within(dialog).getByRole('button', { name: 'Apply refund' })
    expect(confirm).toBeDisabled()
    await user.type(screen.getByLabelText('Reason (required)'), 'chargeback')
    expect(confirm).toBeDisabled()
    await user.type(screen.getByLabelText('Refund amount in USD'), '40')
    await waitFor(() => expect(confirm).toBeEnabled())
    await user.click(confirm)
    await waitFor(() => expect(api.refundTopup).toHaveBeenCalled())
    expect(api.refundTopup.mock.calls[0][0]).toBe('EPY7001')
    expect(api.refundTopup.mock.calls[0][1]).toBe('chargeback')
  })
})
