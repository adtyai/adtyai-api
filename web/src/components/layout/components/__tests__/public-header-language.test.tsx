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
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { PublicHeader } from '../public-header'

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'ADTY AI',
    logo: '',
    loading: false,
    logoLoaded: true,
  }),
}))

vi.mock('@/hooks/use-top-nav-links', () => ({
  useTopNavLinks: () => [],
}))

vi.mock('@/hooks/use-notifications', () => ({
  useNotifications: () => ({
    popoverOpen: false,
    setPopoverOpen: () => {},
    unreadCount: 0,
    activeTab: 'system',
    setActiveTab: () => {},
    notice: '',
    announcements: [],
    loading: false,
  }),
}))

async function renderHeader() {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <PublicHeader
          showThemeSwitch={false}
          showNotifications={false}
          showAuthButtons={false}
        />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('public header language switch', () => {
  it('keeps a language switch in the mobile bar', async () => {
    await renderHeader()

    const mobile = document.querySelector('.lg\\:hidden')
    expect(mobile).not.toBeNull()
    const buttons = screen.getAllByRole('button', { name: 'Change language' })
    expect(buttons.some((button) => mobile?.contains(button))).toBe(true)
  })
})
