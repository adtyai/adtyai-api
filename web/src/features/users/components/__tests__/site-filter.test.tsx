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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as UsersRoute } from '@/routes/_authenticated/users/index'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UsersProvider } from '../users-provider'
import { UsersTable } from '../users-table'

const clients: QueryClient[] = []
const people = [
  { id: 2, username: 'main-customer', site_id: 0, site_name: 'Main Studio' },
  { id: 3, username: 'north-customer', site_id: 7, site_name: 'North Studio' },
  { id: 4, username: 'south-customer', site_id: 8, site_name: 'South Studio' },
].map((person) => ({
  display_name: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  created_at: 1735689600,
  ...person,
}))

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

async function renderUsers(initialEntry = '/users/') {
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/user/sites') {
      return {
        data: {
          success: true,
          data: [
            { site_id: 0, site_name: 'Main Studio' },
            { site_id: 7, site_name: 'North Studio' },
            { site_id: 8, site_name: 'South Studio' },
          ],
        },
      }
    }
    const request = new URL(url, 'https://console.example.test')
    if (url === '/api/user/') {
      Object.entries(config?.params ?? {}).forEach(([key, value]) => {
        if (value !== undefined) request.searchParams.set(key, String(value))
      })
    } else if (!url.startsWith('/api/user/search?')) {
      throw new Error(`Unexpected GET ${url}`)
    }
    const siteID = request.searchParams.get('site_id')
    const items =
      siteID === null
        ? people
        : people.filter((person) => String(person.site_id) === siteID)
    return { data: { success: true, data: { items, total: items.length } } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const users = createRoute({
    getParentRoute: () => auth,
    path: 'users/',
    validateSearch: UsersRoute.options.validateSearch,
    component: () => (
      <TooltipProvider>
        <UsersProvider>
          <UsersTable />
        </UsersProvider>
      </TooltipProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([users])]),
    history: createMemoryHistory({ initialEntries: [initialEntry] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

function siteFilter() {
  const trigger = screen
    .getAllByRole('button', { name: /^Site/ })
    .find((button) => button.getAttribute('aria-haspopup') === 'dialog')
  expect(trigger).toBeDefined()
  return trigger as HTMLElement
}

it('keeps the site ID and name when a user response is parsed', () => {
  expect(userSchema.parse(people[1])).toMatchObject({
    site_id: 7,
    site_name: 'North Studio',
  })
})

it('shows the site name and ID, filters the main site as zero, and restores all sites when cleared', async () => {
  const user = userEvent.setup()
  const router = await renderUsers()
  expect(await screen.findByText('north-customer')).toBeVisible()
  expect(screen.getByText('North Studio')).toBeVisible()
  expect(screen.getAllByText('Site ID: 7').length).toBeGreaterThan(0)

  await user.click(siteFilter())
  await user.click(
    await screen.findByRole('option', { name: /Main Studio.*0/ })
  )

  await waitFor(() =>
    expect(screen.queryByText('north-customer')).not.toBeInTheDocument()
  )
  expect(screen.getByText('main-customer')).toBeVisible()
  expect(router.state.location.search).toMatchObject({ site_id: ['0'] })

  await user.click(screen.getByRole('option', { name: 'Clear filters' }))
  expect(await screen.findByText('north-customer')).toBeVisible()
  expect(router.state.location.search).not.toHaveProperty('site_id')
})

it('restores a site from the URL and resets the page when the site changes while retaining status', async () => {
  const user = userEvent.setup()
  const router = await renderUsers(
    '/users/?site_id=%5B%227%22%5D&status=%5B%221%22%5D&page=2&pageSize=1'
  )
  expect(await screen.findByText('north-customer')).toBeVisible()
  expect(screen.queryByText('south-customer')).not.toBeInTheDocument()

  await user.click(siteFilter())
  await user.click(
    await screen.findByRole('option', { name: /South Studio.*8/ })
  )

  expect(await screen.findByText('south-customer')).toBeVisible()
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      site_id: ['8'],
      status: ['1'],
    })
  )
  expect(router.state.location.search.page ?? 1).toBe(1)
})
