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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApiKey } from '@/features/keys/types'
import { api } from '@/lib/api'

import { AdtyQuickStartCard } from '../adty-quick-start-card'

let client: QueryClient
let serverAddress: string
let siteBaseUrl: string

const appKey = { id: 7, name: 'App key', key: 'masked', status: 1 } as ApiKey

beforeEach(() => {
  window.localStorage.clear()
  serverAddress = 'https://status.example.test'
  siteBaseUrl = ''
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return {
        data: { success: true, data: { server_address: serverAddress } },
      }
    }
    if (url === '/api/adty/site') {
      return { data: { success: true, data: { api_base_url: siteBaseUrl } } }
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: ['gpt-5.4'] } }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

async function renderCard(apiKey: ApiKey | null = appKey) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => <AdtyQuickStartCard apiKey={apiKey} model='gpt-5.4' />,
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('quick start card', () => {
  it('shows the site API base URL and keeps it when the client changes', async () => {
    siteBaseUrl = 'https://api.example.test/'
    const user = userEvent.setup()
    await renderCard()
    const baseUrl = await screen.findByTestId('adty-base-url')
    await waitFor(() =>
      expect(baseUrl).toHaveTextContent(/^https:\/\/api\.example\.test$/)
    )

    for (const name of ['Codex CLI', 'OpenAI SDK', 'Cherry Studio']) {
      await user.click(screen.getByRole('tab', { name }))
      expect(screen.getByRole('tab', { name })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(baseUrl).toHaveTextContent(/^https:\/\/api\.example\.test$/)
    }
  })

  it('falls back to the status server address when the site has no API base URL', async () => {
    await renderCard()

    await vi.waitFor(() =>
      expect(screen.getByTestId('adty-base-url')).toHaveTextContent(
        'https://status.example.test'
      )
    )
  })

  it('fetches the real key and opens CC Switch with the shown base URL', async () => {
    siteBaseUrl = 'https://api.example.test'
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true, data: { key: 'real' } } })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const user = userEvent.setup()
    await renderCard()

    await user.click(
      screen.getByRole('button', { name: 'Import to CC Switch' })
    )
    expect(post).toHaveBeenCalledWith('/api/token/7/key')
    const dialog = await screen.findByRole('dialog', {
      name: 'Import to CC Switch',
    })
    expect(dialog).toBeVisible()
    await user.click(screen.getByRole('combobox', { name: 'Primary Model' }))
    await user.click(await screen.findByRole('option', { name: 'gpt-5.4' }))
    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    const url = new URL(String(open.mock.calls[0][0]))
    expect(url.searchParams.get('endpoint')).toBe('https://api.example.test')
    expect(url.searchParams.get('apiKey')).toBe('sk-real')
  })

  it('offers only key creation when the user has no API key yet', async () => {
    await renderCard(null)

    expect(
      await screen.findByRole('button', { name: 'Create API Key' })
    ).toHaveAttribute('href', '/keys')
    expect(
      screen.queryByRole('button', { name: 'Import to CC Switch' })
    ).not.toBeInTheDocument()
  })
})
