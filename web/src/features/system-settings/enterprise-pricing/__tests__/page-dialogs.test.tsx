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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { EnterprisePricingSettings } from '../index'
import type { EnterprisePriceRule } from '../types'

const sampleRow: EnterprisePriceRule = {
  id: 42,
  user_id: 7,
  group_code: 'vip',
  model_name: 'gpt-4o',
  discount: 0.85,
  expires_at: null,
  created_by: 1,
  updated_at: '2026-10-01T00:00:00Z',
}

const { listEnterprisePricing } = vi.hoisted(() => ({
  listEnterprisePricing: vi.fn(),
}))

vi.mock('../api', () => ({
  listEnterprisePricing,
  createEnterprisePricing: vi.fn(),
  updateEnterprisePricing: vi.fn(),
  deleteEnterprisePricing: vi.fn(),
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={client}>
      <EnterprisePricingSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  listEnterprisePricing.mockResolvedValue({
    rows: [sampleRow],
    total: 1,
  })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('EnterprisePricingSettings dialogs', () => {
  it('mounts RuleFormDialog when Create rule is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('42')
    await user.click(screen.getByRole('button', { name: 'Create rule' }))

    expect(
      await screen.findByRole('heading', {
        name: 'Create enterprise pricing rule',
      })
    ).toBeInTheDocument()
  })

  it('mounts RuleFormDialog when Edit is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('42')
    await user.click(screen.getByRole('button', { name: 'Edit' }))

    expect(
      await screen.findByRole('heading', {
        name: 'Edit enterprise pricing rule',
      })
    ).toBeInTheDocument()
  })

  it('mounts ConfirmDeleteDialog when Delete is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('42')
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    expect(
      await screen.findByRole('heading', {
        name: 'Delete enterprise pricing rule',
      })
    ).toBeInTheDocument()
  })
})
