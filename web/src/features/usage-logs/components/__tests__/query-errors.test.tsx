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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { UsageLogsProvider } from '../usage-logs-provider'
import { UsageLogsTable } from '../usage-logs-table'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('shows a retryable error instead of empty usage logs after GET fails', async () => {
  let failed = true
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.includes('/log/')) {
      if (failed) throw new Error('offline')
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    return { data: { success: true, data: {} } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => (
      <UsageLogsProvider>
        <UsageLogsTable logCategory='common' />
      </UsageLogsProvider>
    ),
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(screen.queryByText('No Logs Found')).not.toBeInTheDocument()
  failed = false
  await userEvent.setup().click(retry)

  expect(await screen.findByText('No Logs Found')).toBeInTheDocument()
  client.clear()
})
