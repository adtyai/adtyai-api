/*
Copyright (C) 2023-2026 QuantumNous
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OrdersSettings } from '../index'
import type { TopupOrder } from '../types'

const sample: TopupOrder = {
  id: 1,
  order_no: 'ORD-TEST-1',
  user_id: 9,
  username: 'alice',
  amount_usd: 10,
  paid_amount_usd: 10,
  credit_amount_usd: 10,
  bonus_amount_usd: 0,
  status: 'pending',
  refund_status: 'none',
  refunded_amount: 0,
  create_time: 1700000000,
  complete_time: 0,
  payment_method: 'epay',
}

const { listAdminOrders, getAdminOrder, completeAdminOrder } = vi.hoisted(
  () => ({
    listAdminOrders: vi.fn(),
    getAdminOrder: vi.fn(),
    completeAdminOrder: vi.fn(),
  })
)

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: React.ReactNode
    to: string
    className?: string
  }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))

vi.mock('../api', () => ({
  listAdminOrders,
  getAdminOrder,
  completeAdminOrder,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <OrdersSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  listAdminOrders.mockResolvedValue({ rows: [sample], total: 1 })
  getAdminOrder.mockResolvedValue(sample)
})

afterEach(() => {
  vi.clearAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('OrdersSettings ConfirmCompleteDialog', () => {
  it('mounts dialog when Complete is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('ORD-TEST-1')
    await user.click(screen.getByRole('button', { name: 'Complete' }))

    expect(
      await screen.findByRole('heading', { name: 'Complete topup order' })
    ).toBeInTheDocument()
  })
})
