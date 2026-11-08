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
import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { ThemeProvider } from '@/context/theme-provider'
import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Rankings } from '..'
import type { RankingsSnapshot } from '../types'

// VChart depends on canvas; keep the real rankings sections and data queries.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => {} },
}))

const emptySnapshot: RankingsSnapshot = {
  models: [],
  vendors: [],
  top_movers: [],
  top_droppers: [],
  models_history: { points: [], models: [], buckets: 0 },
  vendor_share_history: { points: [], vendors: [], buckets: 0 },
}
type RankingsResult = { data: { success: boolean; data: RankingsSnapshot } }
const clients: QueryClient[] = []
const initialSystemConfig = useSystemConfigStore.getState()

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState(initialSystemConfig)
  localStorage.clear()
})

async function renderRankings(
  request: () => Promise<RankingsResult>,
  cached?: RankingsSnapshot
) {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/rankings') return request()
    if (url === '/api/notice') return { data: { success: true, data: '' } }
    return { data: { success: true, data: {} } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  if (cached) {
    client.setQueryData(['rankings', 'week'], { success: true, data: cached })
  }
  const root = createRootRoute()
  const rankings = createRoute({
    getParentRoute: () => root,
    path: 'rankings/',
    validateSearch: () => ({ period: 'week' }),
    component: Rankings,
  })
  const router = createRouter({
    routeTree: root.addChildren([rankings]),
    history: createMemoryHistory({ initialEntries: ['/rankings/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='light'>
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
  await screen.findByRole('heading', { name: 'Rankings' })
  return client
}

it('keeps the error fallback hidden while the first rankings request is pending', async () => {
  let finish = (_result: RankingsResult) => {}
  const pending = new Promise<RankingsResult>((resolve) => {
    finish = resolve
  })
  await renderRankings(() => pending)

  expect(
    screen.queryByRole('heading', { name: 'Unable to load rankings' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('heading', { name: 'Top Models' })
  ).not.toBeInTheDocument()

  await act(async () => {
    finish({ data: { success: true, data: emptySnapshot } })
  })

  expect(
    await screen.findByRole('heading', { name: 'Top Models' })
  ).toBeVisible()
})

it('shows the server error when the first rankings request fails without a snapshot', async () => {
  await renderRankings(async () => {
    throw new Error('Rankings unavailable')
  })

  expect(
    await screen.findByRole('heading', { name: 'Unable to load rankings' })
  ).toBeVisible()
  expect(screen.getByText('Rankings unavailable')).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Top Models' })
  ).not.toBeInTheDocument()
})

it('renders empty-data sections after a successful rankings response', async () => {
  await renderRankings(async () => ({
    data: { success: true, data: emptySnapshot },
  }))

  expect(
    await screen.findByRole('heading', { name: 'Top Models' })
  ).toBeVisible()
  expect(screen.getByText('No models match the selected filters')).toBeVisible()
  expect(screen.getByText('No vendor data available')).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Unable to load rankings' })
  ).not.toBeInTheDocument()
})

it('keeps cached model data visible when a background rankings refetch fails', async () => {
  const client = await renderRankings(
    async () => {
      throw new Error('Refetch unavailable')
    },
    {
      ...emptySnapshot,
      models: [
        {
          rank: 1,
          model_name: 'cached-model',
          vendor: 'Cached vendor',
          category: 'all',
          total_tokens: 100,
          share: 1,
          growth_pct: 0,
        },
      ],
    }
  )
  expect(await screen.findByText('cached-model')).toBeVisible()

  await act(async () => {
    await client.refetchQueries({ queryKey: ['rankings', 'week'] })
  })
  await waitFor(() =>
    expect(client.getQueryState(['rankings', 'week'])?.status).toBe('error')
  )

  expect(screen.getByText('cached-model')).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Market Share' })).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Unable to load rankings' })
  ).not.toBeInTheDocument()
})
