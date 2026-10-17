/*
Copyright (C) 2023-2026 QuantumNous
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AffCommission, AffWithdrawal } from '@/features/affiliate/types'

import { AffCommissionsSettings } from '../commissions'
import { AffWithdrawalsSettings } from '../withdrawals'

const withdrawRow: AffWithdrawal = {
  id: 21,
  user_id: 9,
  amount_quota: 1000000,
  fee_quota: 0,
  net_quota: 1000000,
  fee_rate_snapshot: 0,
  min_amount_snapshot: 0,
  idem_key: 'idem-1',
  status: 'pending',
  pay_account: 'alipay',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

const commissionRow: AffCommission = {
  id: 31,
  inviter_id: 2,
  invitee_id: 9,
  order_no: 'ORD-AFF-1',
  source: 'topup',
  paid_amount_usd: 10,
  rate_snapshot: 0.1,
  commission_quota: 100000,
  freeze_days_snapshot: 7,
  status: 'frozen',
  unfreeze_at: '2026-10-08T00:00:00Z',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

const api = vi.hoisted(() => ({
  adminListAffWithdrawals: vi.fn(),
  adminListAffSelfInviteReviews: vi.fn(),
  adminListAffCommissions: vi.fn(),
  adminAffWithdrawAction: vi.fn(),
  adminVoidAffCommission: vi.fn(),
  adminApproveRiskReview: vi.fn(),
  adminRejectRiskReview: vi.fn(),
}))

vi.mock('../api', () => api)

function wrap(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>
  )
}

beforeEach(() => {
  api.adminListAffWithdrawals.mockResolvedValue({
    rows: [withdrawRow],
    total: 1,
  })
  api.adminListAffSelfInviteReviews.mockResolvedValue({ rows: [], total: 0 })
  api.adminListAffCommissions.mockResolvedValue({
    rows: [commissionRow],
    total: 1,
  })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('Aff ConfirmAffActionDialog mount', () => {
  it('withdrawals: mounts dialog when Approve is clicked', async () => {
    const user = userEvent.setup()
    wrap(<AffWithdrawalsSettings />)

    await screen.findByText(String(withdrawRow.id))
    await user.click(screen.getByRole('button', { name: 'Approve' }))

    expect(
      await screen.findByRole('heading', { name: 'Approve withdrawal' })
    ).toBeInTheDocument()
  })

  it('commissions: mounts dialog when Void is clicked', async () => {
    const user = userEvent.setup()
    wrap(<AffCommissionsSettings />)

    await screen.findByText(String(commissionRow.id))
    await user.click(screen.getByRole('button', { name: 'Void' }))

    expect(
      await screen.findByRole('heading', { name: 'Void commission' })
    ).toBeInTheDocument()
  })
})
