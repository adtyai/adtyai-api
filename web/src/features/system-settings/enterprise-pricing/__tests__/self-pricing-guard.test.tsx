/*
Copyright (C) 2023-2026 QuantumNous
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { EnterprisePricingSettings } from '../index'

const { listEnterprisePricing, createEnterprisePricing } = vi.hoisted(() => ({
  listEnterprisePricing: vi.fn(),
  createEnterprisePricing: vi.fn(),
}))

vi.mock('../api', () => ({
  listEnterprisePricing,
  createEnterprisePricing,
  updateEnterprisePricing: vi.fn(),
  deleteEnterprisePricing: vi.fn(),
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <EnterprisePricingSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'root', role: ROLE.SUPER_ADMIN })
  listEnterprisePricing.mockResolvedValue({ rows: [], total: 0 })
})

afterEach(() => {
  vi.clearAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('enterprise self pricing guard', () => {
  it('blocks create when user_id equals current user', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    await screen.findByRole('heading', {
      name: 'Create enterprise pricing rule',
    })

    await user.type(screen.getByLabelText('User ID'), '7')
    await user.clear(screen.getByLabelText('Discount (%)'))
    await user.type(screen.getByLabelText('Discount (%)'), '50')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText('You cannot set enterprise pricing for yourself')
    ).toBeInTheDocument()
    expect(createEnterprisePricing).not.toHaveBeenCalled()
  })
})
