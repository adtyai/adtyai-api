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

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { StatusPage } from '..'

let status: Record<string, unknown>
let uptime: unknown

beforeEach(() => {
  window.localStorage.clear()
  useAuthStore.getState().auth.reset()
  status = { system_name: 'Partner AI', uptime_kuma_enabled: true }
  uptime = {
    success: true,
    data: [
      {
        categoryName: 'Models',
        monitors: [
          // Uptime Kuma's own group names often hold channel or upstream
          // names; the public page must not show them.
          {
            name: 'claude-sonnet-4-5',
            uptime: 0.9995,
            status: 1,
            group: 'aws-channel-03',
          },
          {
            name: 'gpt-4o-mini',
            uptime: 0.97,
            status: 0,
            group: 'azure-east-key2',
          },
        ],
      },
      {
        categoryName: 'Services',
        monitors: [{ name: 'API', uptime: 1, status: 1 }],
      },
    ],
  }
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/status') return { data: { success: true, data: status } }
    if (url === '/api/uptime/status') return { data: uptime }
    if (url === '/api/notice') return { data: { success: true, data: '' } }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

async function renderStatus() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: StatusPage }),
    history: createMemoryHistory({ initialEntries: ['/status'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('public status page', () => {
  it('opens without signing in and lists every monitor by category', async () => {
    await renderStatus()

    const models = await screen.findByRole('region', { name: 'Models' })
    expect(within(models).getByText('claude-sonnet-4-5')).toBeInTheDocument()
    expect(within(models).getByText('gpt-4o-mini')).toBeInTheDocument()
    expect(
      within(screen.getByRole('region', { name: 'Services' })).getByText('API')
    ).toBeInTheDocument()
    expect(useAuthStore.getState().auth.user).toBeNull()
  })

  it('does not show channel names', async () => {
    await renderStatus()

    await screen.findByText('claude-sonnet-4-5')
    expect(document.body.textContent).not.toContain('aws-channel-03')
    expect(document.body.textContent).not.toContain('azure-east-key2')
  })

  it('shows each monitor state and its 24-hour uptime', async () => {
    await renderStatus()

    const up = (await screen.findByText('claude-sonnet-4-5')).closest('li')
    const down = screen.getByText('gpt-4o-mini').closest('li')
    expect(up).toHaveTextContent('Operational')
    expect(up).toHaveTextContent('99.95%')
    expect(down).toHaveTextContent('Down')
    expect(down).toHaveTextContent('97.00%')
  })

  it('reports an outage in the summary when any monitor is down', async () => {
    await renderStatus()

    expect(
      await screen.findByRole('heading', {
        level: 2,
        name: 'Some services are down',
      })
    ).toBeInTheDocument()
  })

  it('reports all clear when every monitor is up', async () => {
    uptime = {
      success: true,
      data: [
        {
          categoryName: 'Services',
          monitors: [{ name: 'API', uptime: 1, status: 1 }],
        },
      ],
    }
    await renderStatus()

    expect(
      await screen.findByRole('heading', {
        level: 2,
        name: 'All systems operational',
      })
    ).toBeInTheDocument()
  })

  it('names the site from /api/status in the page title', async () => {
    await renderStatus()

    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'Partner AI status',
      })
    ).toBeInTheDocument()
  })

  it('falls back to the brand name when /api/status has no site name', async () => {
    status = { uptime_kuma_enabled: true }
    await renderStatus()

    expect(
      await screen.findByRole('heading', { level: 1, name: 'ADTY AI status' })
    ).toBeInTheDocument()
  })

  it('says monitoring is not set up when there are no monitors', async () => {
    uptime = { success: true, data: [] }
    await renderStatus()

    expect(
      await screen.findByText('Service status is not available yet.')
    ).toBeInTheDocument()
  })

  it('does not load monitors when the uptime panel is switched off', async () => {
    status = { system_name: 'Partner AI', uptime_kuma_enabled: false }
    await renderStatus()

    expect(
      await screen.findByText('Service status is not available yet.')
    ).toBeInTheDocument()
    expect(api.get).not.toHaveBeenCalledWith('/api/uptime/status')
  })

  it('shows an error with a retry when the status request fails', async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url === '/api/uptime/status') throw new Error('offline')
      if (url === '/api/notice') return { data: { success: true, data: '' } }
      return { data: { success: true, data: status } }
    })
    await renderStatus()

    expect(
      await screen.findByText('Service status could not be loaded.')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
})
