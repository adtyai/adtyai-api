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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AppHeader } from '@/components/layout/components/app-header'
import { SidebarProvider } from '@/components/ui/sidebar'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

const api = vi.hoisted(() => ({ getSelf: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  getSelf: api.getSelf,
}))

// 1 USD = 500000 quota (the default QuotaPerUnit).
const QUOTA_PER_USD = 500000

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'alice',
    role: 1,
    quota: 12.5 * QUOTA_PER_USD,
  })
})

afterEach(() => {
  api.getSelf.mockReset()
  useAuthStore.getState().auth.reset()
})

async function renderHeader(status: Record<string, unknown> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(STATUS_QUERY_KEY, status)
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <SidebarProvider>
          <AppHeader
            showSearch={false}
            showNotifications={false}
            showConfigDrawer={false}
            showProfileDropdown={false}
          />
        </SidebarProvider>
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/dashboard'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('banner')
  return client
}

describe('header balance', () => {
  it('shows the balance from /api/user/self in the display currency', async () => {
    api.getSelf.mockResolvedValue({
      success: true,
      data: { quota: 128.5 * QUOTA_PER_USD },
    })

    await renderHeader()

    expect(await screen.findByText('$128.50')).toBeInTheDocument()
    expect(screen.getByText('Balance')).toBeInTheDocument()
  })

  it('rounds the balance to cents', async () => {
    api.getSelf.mockResolvedValue({
      success: true,
      data: { quota: 0.0512 * QUOTA_PER_USD },
    })

    await renderHeader()

    expect(await screen.findByText('$0.05')).toBeInTheDocument()
  })

  it('keeps showing the session balance when /api/user/self fails', async () => {
    api.getSelf.mockRejectedValue(new Error('network down'))

    await renderHeader()

    expect(await screen.findByText('$12.50')).toBeInTheDocument()
    expect(api.getSelf).toHaveBeenCalled()
  })

  it('links the add funds button to the wallet', async () => {
    api.getSelf.mockResolvedValue({ success: true, data: { quota: 0 } })

    await renderHeader()

    // Base UI's Button keeps role=button on the rendered router link.
    expect(screen.getByRole('button', { name: 'Add funds' })).toHaveAttribute(
      'href',
      '/wallet'
    )
  })

  it('refreshes the balance when the wallet invalidates its top-up queries', async () => {
    api.getSelf.mockResolvedValue({
      success: true,
      data: { quota: 1 * QUOTA_PER_USD },
    })
    const client = await renderHeader()
    await screen.findByText('$1.00')

    api.getSelf.mockResolvedValue({
      success: true,
      data: { quota: 51 * QUOTA_PER_USD },
    })
    await act(() => client.invalidateQueries({ queryKey: ['adty-topup'] }))

    expect(await screen.findByText('$51.00')).toBeInTheDocument()
  })
})

describe('header navigation', () => {
  it('shows the Affiliate Program link to /affiliate by default', async () => {
    api.getSelf.mockResolvedValue({ success: true, data: { quota: 0 } })

    await renderHeader()

    const banner = screen.getByRole('banner')
    expect(
      within(banner).getByRole('link', { name: 'Affiliate Program' })
    ).toHaveAttribute('href', '/affiliate')
  })

  it('hides the Affiliate Program link when the admin turns it off', async () => {
    api.getSelf.mockResolvedValue({ success: true, data: { quota: 0 } })

    await renderHeader({
      HeaderNavModules: JSON.stringify({ affiliate: false }),
    })

    const banner = screen.getByRole('banner')
    expect(
      within(banner).queryByRole('link', { name: 'Affiliate Program' })
    ).toBeNull()
    expect(
      within(banner).getByRole('link', { name: 'Docs' })
    ).toBeInTheDocument()
  })
})
