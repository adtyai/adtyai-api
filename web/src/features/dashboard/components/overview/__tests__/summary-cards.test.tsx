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
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { QuotaDataItem } from '@/features/dashboard/types'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { SummaryCards } from '../summary-cards'

// Default currency config: 500000 quota units = 1 USD.
const USD = 500000
const HOUR = 3600
const DAY = 24 * HOUR

let client: QueryClient
let rows: QuotaDataItem[]
let requestedRange: { start_timestamp?: number; end_timestamp?: number }

function at(date: Date): number {
  return Math.floor(date.getTime() / 1000)
}

beforeEach(() => {
  window.localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  rows = []
  requestedRange = {}
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/status') {
      return { data: { success: true, data: { display_in_currency: true } } }
    }
    if (url === '/api/data/self') {
      requestedRange = config?.params ?? {}
      return { data: { success: true, data: rows } }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.useRealTimers()
  vi.restoreAllMocks()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
})

async function renderCards(balanceUsd: number) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'u', role: 1, quota: balanceUsd * USD })
  const router = createRouter({
    routeTree: createRootRoute({ component: SummaryCards }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

function cardFor(title: string): HTMLElement {
  const card = screen.getByText(title).closest('.group')
  if (!(card instanceof HTMLElement)) throw new Error(`no card ${title}`)
  return card
}

describe('overview summary cards', () => {
  it('estimates about 10 days left from a 30 balance and a 7-day daily average of 3', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    const now = new Date(2026, 9, 20, 12, 0, 0)
    vi.setSystemTime(now)
    const nowTs = at(now)
    rows = [
      { created_at: nowTs - 2 * HOUR, quota: 1.5 * USD, count: 1 },
      { created_at: nowTs - 3 * DAY, quota: 19.5 * USD, count: 1 },
      // Earlier this month but outside the 7-day average.
      { created_at: nowTs - 12 * DAY, quota: 90 * USD, count: 1 },
    ]

    await renderCards(30)

    expect(await screen.findByText('~10 days')).toBeInTheDocument()
    expect(requestedRange.start_timestamp).toBe(at(new Date(2026, 9, 1)))
  })

  it('splits one response into month-to-date spend and today’s requests and tokens', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    // Early in the month the 7-day window starts in the previous month.
    const now = new Date(2026, 10, 2, 12, 0, 0)
    vi.setSystemTime(now)
    const nowTs = at(now)
    const todayStart = at(new Date(2026, 10, 2))
    rows = [
      {
        created_at: todayStart + HOUR,
        quota: 2 * USD,
        count: 40,
        token_used: 1200,
      },
      {
        created_at: todayStart - 2 * HOUR,
        quota: 3 * USD,
        count: 7,
        token_used: 900,
      },
      // Previous month: counts for Runway only.
      { created_at: nowTs - 4 * DAY, quota: 9 * USD, count: 5 },
    ]

    await renderCards(100)

    // Today's $2 plus yesterday's $3; the $9 from last month is left out.
    expect(
      await within(cardFor('Spent this month')).findByText('$5')
    ).toBeVisible()
    expect(within(cardFor('Requests today')).getByText('40')).toBeVisible()
    expect(within(cardFor('Tokens today')).getByText('1.2K')).toBeVisible()
    expect(requestedRange.start_timestamp).toBe(nowTs - 7 * DAY)
    // Daily average (2 + 3 + 9) / 7 = 2 → 100 / 2 = 50 days.
    expect(screen.getByText('~50 days')).toBeInTheDocument()
  })
})
