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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { CustomerSheet } from '@/features/adty-site-owner/components/customer-sheet'
import { OwnerVerificationProvider } from '@/features/adty-site-owner/components/owner-verification'
import { PromoCodesPanel } from '@/features/adty-site-owner/components/promo-codes-panel'
import { api } from '@/lib/api'

import { PromoPanel } from '../components/promo-panel'

const clients: QueryClient[] = []
function mount(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>{node}</OwnerVerificationProvider>
    </QueryClientProvider>
  )
}
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

it.each([
  { name: 'platform promo', node: <PromoPanel canCreate canWrite /> },
  { name: 'owner promo', node: <PromoCodesPanel /> },
])('preserves an edited $name dialog when Stay is chosen', async ({ node }) => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  const user = userEvent.setup()
  mount(node)
  await user.click(
    await screen.findByRole('button', { name: 'New promo code' })
  )
  await user.type(screen.getByLabelText('Code'), 'DRAFT001')
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Stay' }))
  expect(screen.getByLabelText('Code')).toHaveValue('DRAFT001')
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Leave' }))
  await waitFor(() =>
    expect(screen.queryByLabelText('Code')).not.toBeInTheDocument()
  )
})

it('confirms before closing an owner customer drawer containing an unsent trial amount', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: String(url).endsWith('/customers/1')
        ? {
            customer: { id: 1, username: 'alice', created_at: 1 },
            prices: [],
            tokens: [],
          }
        : { items: [], total: 0 },
    },
  }))
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  mount(<CustomerSheet customerId={1} onOpenChange={onOpenChange} />)
  await user.type(await screen.findByLabelText('Amount (USD)'), '1.25')
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Stay' }))
  expect(screen.getByLabelText('Amount (USD)')).toHaveValue(1.25)
  expect(onOpenChange).not.toHaveBeenCalled()
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Leave' }))
  expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
})
