/*
Copyright (C) 2023-2026 QuantumNous
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { BalanceAdjustmentsSettings } from '../index'
import type { BalanceAdjustRequest } from '../types'

const sample: BalanceAdjustRequest = {
  id: 41,
  user_id: 9,
  account: 'quota',
  amount: 1_000_000,
  reason: 'test adjust',
  status: 'pending',
  applicant_id: 2,
  created_at: '2026-10-01T00:00:00Z',
}

const api = vi.hoisted(() => ({
  listBalanceAdjustments: vi.fn(),
  createBalanceAdjustment: vi.fn(),
  approveBalanceAdjustment: vi.fn(),
  rejectBalanceAdjustment: vi.fn(),
}))

vi.mock('../api', () => api)

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <BalanceAdjustmentsSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  api.listBalanceAdjustments.mockResolvedValue({ rows: [sample], total: 1 })
})

afterEach(() => {
  vi.clearAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('BalanceAdjustmentsSettings dialogs', () => {
  it('mounts Create dialog when New request is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('41')
    await user.click(screen.getByRole('button', { name: 'New request' }))

    expect(
      await screen.findByRole('heading', { name: 'New balance adjustment' })
    ).toBeInTheDocument()
  })

  it('mounts Confirm dialog when Approve is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('41')
    await user.click(screen.getByRole('button', { name: 'Approve' }))

    expect(
      await screen.findByRole('heading', {
        name: 'Approve balance adjustment',
      })
    ).toBeInTheDocument()
  })
})
