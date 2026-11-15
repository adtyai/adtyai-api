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
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Home } from '@/features/home'
import { api } from '@/lib/api'

let client: QueryClient
let site: Record<string, unknown>
let homePageContent: Promise<unknown>

const SERVER_ADDRESS = 'https://srv.example.test'

// gpt-x: site input 1.5 × 2 = $3 per 1M tokens, official $15 → 80% off.
// no-official: no official price, so it is counted but not compared.
const pricing = {
  success: true,
  data: [
    {
      id: 1,
      model_name: 'gpt-x',
      vendor_id: 1,
      quota_type: 0,
      model_ratio: 1.5,
      completion_ratio: 4,
      enable_groups: ['default'],
      group_ratio: { default: 1 },
    },
    {
      id: 2,
      model_name: 'no-official',
      vendor_id: 1,
      quota_type: 0,
      model_ratio: 1,
      completion_ratio: 1,
      enable_groups: ['default'],
      group_ratio: { default: 1 },
    },
  ],
  vendors: [{ id: 1, name: 'OpenAI' }],
  group_ratio: { default: 1 },
  usable_group: { default: { desc: 'Default', ratio: 1 } },
  supported_endpoint: {},
  auto_groups: [],
}

const officialPrices = {
  'gpt-x': {
    input: '15',
    output: '60',
    cache_read: null,
    cache_write: null,
    per_call: null,
  },
}

beforeEach(() => {
  window.localStorage.clear()
  site = { api_base_url: 'https://api.example.test/', min_topup: 5 }
  homePageContent = Promise.resolve({ success: true, data: '' })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    switch (url) {
      case '/api/status':
        return {
          data: { success: true, data: { server_address: SERVER_ADDRESS } },
        }
      case '/api/adty/site':
        return { data: { success: true, data: site } }
      case '/api/pricing':
        return { data: pricing }
      case '/api/adty/pricing/official':
        return {
          data: {
            success: true,
            data: { prices: officialPrices, updated_at: 1 },
          },
        }
      case '/api/adty/referral/public-settings':
        return {
          data: { success: true, data: { enabled: true, default_rate: 10 } },
        }
      case '/api/home_page_content':
        return { data: await homePageContent }
      case '/api/notice':
        return { data: { success: true, data: '' } }
      default:
        throw new Error(`Unexpected request: ${url}`)
    }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

async function renderHome() {
  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('landing page', () => {
  it('renders the hero, the migration block, the endpoints table, the referral strip and the closing card', async () => {
    await renderHome()

    const hero = await screen.findByRole('heading', { level: 1 })
    expect(hero).toHaveTextContent('One endpoint.')
    expect(hero).toHaveTextContent('Every model.')
    expect(
      screen.getByRole('region', { name: /It’s one line\./ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: /Four shapes\./ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: /Start with one request\./ })
    ).toBeInTheDocument()
    expect(
      await screen.findByRole('region', {
        name: 'Invite friends, earn on every top-up',
      })
    ).toBeInTheDocument()
    expect(screen.getByRole('contentinfo')).toBeInTheDocument()
  })

  it('drops the blocks the minimal layout replaced', async () => {
    await renderHome()

    await screen.findByRole('heading', { level: 1 })
    for (const name of [
      'Popular model prices',
      'Start in three steps',
      'Setup guides',
      'Why ADTY AI',
      'Frequently asked questions',
      'Payment methods',
    ]) {
      expect(screen.queryByRole('region', { name })).toBeNull()
    }
  })

  it('puts the site API address into the code card', async () => {
    await renderHome()

    const code = await screen.findByTestId('landing-code')
    await waitFor(() =>
      expect(code).toHaveTextContent(
        'export ANTHROPIC_BASE_URL=https://api.example.test'
      )
    )
  })

  it('falls back to the status server address when the site address is empty', async () => {
    site = { api_base_url: '', min_topup: 5 }
    await renderHome()

    const code = await screen.findByTestId('landing-code')
    await waitFor(() =>
      expect(code).toHaveTextContent(`ANTHROPIC_BASE_URL=${SERVER_ADDRESS}`)
    )
  })

  it('lists the four endpoint shapes on the site address', async () => {
    await renderHome()

    const endpoints = await screen.findByRole('region', {
      name: /Four shapes\./,
    })
    expect(
      within(endpoints).getAllByText('https://api.example.test/v1')
    ).toHaveLength(2)
    expect(
      within(endpoints).getAllByText('https://api.example.test')
    ).toHaveLength(2)
    expect(within(endpoints).getByText('x-goog-api-key')).toBeInTheDocument()
    expect(
      within(endpoints).getByText('Anthropic Messages')
    ).toBeInTheDocument()
  })

  it('leaves the upstream attribution, source link and support email out of the footer', async () => {
    await renderHome()

    const footer = await screen.findByRole('contentinfo')
    const hrefs = within(footer)
      .getAllByRole('link')
      .map((link) => link.getAttribute('href'))
    expect(hrefs).not.toContain('https://github.com/QuantumNous/new-api')
    expect(hrefs).not.toContain('https://github.com/adtyai/adtyai-api')
    expect(hrefs).not.toContain('mailto:support@adtyai.com')
    expect(hrefs).not.toContain('/pricing')
    expect(within(footer).queryByText(/New API/)).toBeNull()
    expect(within(footer).queryByText('support@adtyai.com')).toBeNull()
  })

  it('shows the landing page instead of a loading text while the custom home content loads', async () => {
    homePageContent = new Promise(() => {})
    await renderHome()

    expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(
      'One endpoint.'
    )
    expect(screen.queryByText('Loading...')).toBeNull()
  })

  it('links inside the site with plain anchors', async () => {
    await renderHome()

    const hero = await screen.findByRole('region', {
      name: /One endpoint\./,
    })
    expect(
      within(hero).getByRole('link', { name: 'Get a key' })
    ).toHaveAttribute('href', '/sign-up')
    expect(
      within(hero).getByRole('link', { name: 'Read the docs' })
    ).toHaveAttribute('href', '/docs')

    const closing = screen.getByRole('region', {
      name: /Start with one request\./,
    })
    expect(
      within(closing).getByRole('link', { name: 'See pricing' })
    ).toHaveAttribute('href', '/pricing')
  })
})
