/*
Copyright (C) 2023-2026 QuantumNous
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OrderRefundsSettings } from '../index'
import type { TopupOrder } from '../types'

const sample: TopupOrder = {
  id: 1,
  order_no: 'ORD-REF-1',
  user_id: 9,
  amount_usd: 10,
  paid_amount_usd: 10,
  credit_amount_usd: 10,
  bonus_amount_usd: 0,
  status: 'paid',
  refund_status: 'none',
  refunded_amount: 0,
  create_time: 1700000000,
  complete_time: 1700000100,
  payment_method: 'epay',
}

const { getTopupOrderByNo, adminRefundTopupOrder } = vi.hoisted(() => ({
  getTopupOrderByNo: vi.fn(),
  adminRefundTopupOrder: vi.fn(),
}))

vi.mock('../api', () => ({
  getTopupOrderByNo,
  adminRefundTopupOrder,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <OrderRefundsSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  getTopupOrderByNo.mockResolvedValue(sample)
})

afterEach(() => {
  vi.clearAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('OrderRefundsSettings ConfirmRefundDialog', () => {
  it('mounts dialog when Refund is clicked (root)', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(
      screen.getByPlaceholderText('Enter order number'),
      'ORD-REF-1'
    )
    await user.click(screen.getByRole('button', { name: 'Search' }))

    await screen.findByText('ORD-REF-1')
    await user.click(screen.getByRole('button', { name: 'Refund' }))

    expect(
      await screen.findByRole('heading', { name: 'Refund order' })
    ).toBeInTheDocument()
  })

  it('disables refund for non-root', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 3, username: 'bob', role: ROLE.ADMIN })
    const user = userEvent.setup()
    renderPage()

    await user.type(
      screen.getByPlaceholderText('Enter order number'),
      'ORD-REF-1'
    )
    await user.click(screen.getByRole('button', { name: 'Search' }))

    await screen.findByText('ORD-REF-1')
    expect(
      screen.getByRole('button', { name: 'Root only' })
    ).toBeDisabled()
    expect(
      screen.queryByRole('heading', { name: 'Refund order' })
    ).not.toBeInTheDocument()
  })
})
