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
  it('renders the hero, the blocks below it and the footer', async () => {
    await renderHome()

    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'Claude, GPT and Gemini with one API key',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Payment methods' })
    ).toBeInTheDocument()
    for (const name of [
      'Popular model prices',
      'Start in three steps',
      'Setup guides',
      'Why ADTY AI',
      'Frequently asked questions',
    ]) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument()
    }
    expect(
      screen.getByText(
        'Set ANTHROPIC_BASE_URL to this site, without /v1, and use your key.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Send your first request today' })
    ).toBeNull()
    expect(
      screen.queryByText(
        'After signing up, copy the base URL and a key from the console and you are ready.'
      )
    ).toBeNull()
    expect(
      await screen.findByRole('region', {
        name: 'Invite friends, earn on every top-up',
      })
    ).toBeInTheDocument()
    expect(screen.getByRole('contentinfo')).toBeInTheDocument()
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

  it('computes the stats strip from the prices and the minimum top-up', async () => {
    await renderHome()

    const stats = await screen.findByTestId('landing-stats')
    await waitFor(() => expect(stats).toHaveTextContent('80%'))
    expect(within(stats).getByText('2')).toBeInTheDocument()
    expect(stats).toHaveTextContent(/\$5/)
  })

  it('shows savings only for models with an official price', async () => {
    await renderHome()

    const table = await screen.findByRole('region', {
      name: 'Popular model prices',
    })
    expect(await within(table).findByText('gpt-x')).toBeInTheDocument()
    expect(within(table).queryByText('no-official')).toBeNull()
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
      'Claude, GPT and Gemini with one API key'
    )
    expect(screen.queryByText('Loading...')).toBeNull()
  })

  it('links inside the site with plain anchors', async () => {
    await renderHome()

    const hero = await screen.findByRole('region', {
      name: 'Claude, GPT and Gemini with one API key',
    })
    expect(
      within(hero).getByRole('link', { name: 'View pricing' })
    ).toHaveAttribute('href', '/pricing')
  })
})
