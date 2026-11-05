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
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import { adtySiteQueryKey } from '@/features/adty-site/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Footer } from '../footer'
import { PublicLayout } from '../public-layout'

const REMOVED_HREFS = [
  'https://github.com/QuantumNous/new-api',
  'https://github.com/adtyai/adtyai-api',
  '/pricing',
  '/docs',
  '/affiliate',
  'mailto:support@adtyai.com',
]

const initialSystemConfig = useSystemConfigStore.getState().config

afterEach(() => {
  useSystemConfigStore.setState({ config: initialSystemConfig })
})

async function renderPage(
  ui: ReactNode,
  status: Record<string, unknown> = {},
  site: Record<string, unknown> = {}
): Promise<void> {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(STATUS_QUERY_KEY, status)
  client.setQueryData(adtySiteQueryKey, site)
  client.setQueryData(['notice'], { success: true, data: '' })
  const router = createRouter({
    routeTree: createRootRoute({ component: () => ui }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('contentinfo')
}

function footerHrefs(): (string | null)[] {
  return within(screen.getByRole('contentinfo'))
    .getAllByRole('link')
    .map((link) => link.getAttribute('href'))
}

describe('footer', () => {
  it('omits the upstream attribution, source link and product columns', async () => {
    await renderPage(<Footer />)

    const footer = screen.getByRole('contentinfo')
    for (const href of REMOVED_HREFS) {
      expect(footerHrefs()).not.toContain(href)
    }
    expect(within(footer).queryByText(/New API/)).toBeNull()
    expect(within(footer).queryByText('support@adtyai.com')).toBeNull()
  })

  it('keeps custom footer HTML without the removed links', async () => {
    useSystemConfigStore.getState().setConfig({
      footerHtml: '<p>Custom footer</p>',
    })

    await renderPage(<Footer />)

    const footer = screen.getByRole('contentinfo')
    expect(within(footer).getByText('Custom footer')).toBeInTheDocument()
    for (const href of REMOVED_HREFS) {
      expect(footerHrefs()).not.toContain(href)
    }
  })

  it('lists only the legal documents that are switched on', async () => {
    await renderPage(<Footer />, {
      user_agreement_enabled: true,
      privacy_policy_enabled: false,
    })

    const footer = screen.getByRole('contentinfo')
    expect(
      within(footer).getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    expect(
      within(footer).queryByRole('link', { name: 'Privacy Policy' })
    ).toBeNull()
  })

  it('lists the refund policy when that site setting is on', async () => {
    await renderPage(
      <Footer />,
      { user_agreement_enabled: true },
      { refund_policy_enabled: true }
    )

    expect(
      within(screen.getByRole('contentinfo')).getByRole('link', {
        name: 'Refund Policy',
      })
    ).toHaveAttribute('href', '/refund-policy')
  })

  it('leaves legal links out when no legal document is switched on', async () => {
    await renderPage(<Footer />)

    expect(screen.queryByRole('link', { name: 'User Agreement' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Privacy Policy' })).toBeNull()
  })
})

describe('public layout', () => {
  it('renders the footer below the page content without the upstream link', async () => {
    await renderPage(
      <PublicLayout>
        <p>Docs page</p>
      </PublicLayout>
    )

    expect(screen.getByText('Docs page')).toBeInTheDocument()
    expect(screen.getAllByRole('contentinfo')).toHaveLength(1)
    expect(footerHrefs()).not.toContain(
      'https://github.com/QuantumNous/new-api'
    )
  })

  it('renders the footer when the page manages its own container', async () => {
    await renderPage(
      <PublicLayout showMainContainer={false}>
        <p>Pricing page</p>
      </PublicLayout>
    )

    expect(screen.getAllByRole('contentinfo')).toHaveLength(1)
  })
})
