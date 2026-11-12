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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { CustomerSheet } from '../components/customer-sheet'
import { CustomersPanel } from '../components/customers-panel'
import { EarningsPanel } from '../components/earnings-panel'
import { OverviewPanel } from '../components/overview-panel'
import { OwnerVerificationProvider } from '../components/owner-verification'
import { SiteSettingsPanel } from '../components/site-settings-panel'
import type { OwnerSite } from '../types'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('preserves an edited brand when another site setting refreshes server data', async () => {
  const site: OwnerSite = {
    id: 1,
    status: 'active',
    subdomain: 'alpha',
    root_domain: 'example',
    host: 'alpha.example',
    base_url: 'https://alpha.example',
    cost_factor: '1',
    markup_min: '1',
    markup_max: '2',
    markup_increase_delay_hours: 24,
    custom_domain_enabled: false,
    sender_domain_enabled: false,
    brand: {
      name: 'Original name',
      logo_url: '',
      tagline: '',
      notice: '',
      support_contact: '',
    },
    bonus_rates: {},
    seo_index: false,
    agreement_version: '1',
    agreement_at: 1,
    prices: [],
    pricing_version: 1,
    domains: [],
    visibility: {},
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const node = (current: OwnerSite) => (
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>
        <SiteSettingsPanel site={current} />
      </OwnerVerificationProvider>
    </QueryClientProvider>
  )
  const view = render(node(site))
  const refreshed = {
    ...site,
    brand: { ...site.brand, name: 'Refreshed name' },
  }
  view.rerender(node(refreshed))
  expect(screen.getByLabelText('Site name')).toHaveValue('Refreshed name')
  const user = userEvent.setup()
  await user.clear(screen.getByLabelText('Site name'))
  await user.type(screen.getByLabelText('Site name'), 'Unsaved name')
  view.rerender(
    node({ ...refreshed, seo_index: true, brand: { ...refreshed.brand } })
  )
  expect(screen.getByLabelText('Site name')).toHaveValue('Unsaved name')
  client.clear()
})

it.each([
  {
    name: 'customers',
    node: <CustomersPanel />,
    data: { items: [], total: 0 },
    ready: 'No customers yet',
  },

  {
    name: 'earnings',
    node: <EarningsPanel />,
    data: { balances: {}, page: { items: [], total: 0 } },
    ready: 'No earnings yet',
  },
  {
    name: 'overview',
    node: <OverviewPanel />,
    data: {
      series: [],
      total: {},
      balances: {},
      page: { items: [], total: 0 },
    },
    ready: 'No data yet',
  },
])('recovers $name from a failed GET with an explicit retry', async (entry) => {
  let failed = true
  vi.spyOn(api, 'get').mockImplementation(async () => {
    if (failed) throw new Error('offline')
    return { data: { success: true, data: entry.data } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>{entry.node}</OwnerVerificationProvider>
    </QueryClientProvider>
  )
  const retry = await screen.findByRole('button', { name: 'Retry' })
  failed = false
  await userEvent.setup().click(retry)

  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  )
  expect(await screen.findByText(entry.ready)).toBeInTheDocument()
  client.clear()
})

it.each(['detail', 'logs', 'topups'])(
  'retries failed customer %s without claiming the data is hidden',
  async (section) => {
    let failed = true
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      const isDetail = path.endsWith('/customers/1')
      if (
        failed &&
        (section === 'detail' ? isDetail : path.endsWith(`/${section}`))
      ) {
        throw new Error('offline')
      }
      return {
        data: {
          success: true,
          data: isDetail
            ? {
                customer: { id: 1, username: 'alice', created_at: 1 },
                prices: [],
                tokens: [],
              }
            : { items: [], total: 0 },
        },
      }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <OwnerVerificationProvider>
          <CustomerSheet customerId={1} onOpenChange={() => {}} />
        </OwnerVerificationProvider>
      </QueryClientProvider>
    )
    if (section === 'topups') {
      await screen.findByText('alice')
      await userEvent
        .setup()
        .click(screen.getByRole('tab', { name: 'Top-ups' }))
    }
    const retry = await screen.findByRole('button', { name: 'Retry' })
    expect(
      screen.queryByText('The platform does not show this information')
    ).not.toBeInTheDocument()
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    client.clear()
  }
)

it('does not offer to add a sender domain when its GET failed', async () => {
  let failed = true
  vi.spyOn(api, 'get').mockImplementation(async () => {
    if (failed) throw new Error('offline')
    return { data: { success: true, data: null } }
  })
  const site: OwnerSite = {
    id: 1,
    status: 'active',
    subdomain: 'alpha',
    root_domain: 'example',
    host: 'alpha.example',
    base_url: 'https://alpha.example',
    cost_factor: '1',
    markup_min: '1',
    markup_max: '2',
    markup_increase_delay_hours: 24,
    custom_domain_enabled: false,
    sender_domain_enabled: true,
    brand: {
      name: '',
      logo_url: '',
      tagline: '',
      notice: '',
      support_contact: '',
    },
    bonus_rates: {},
    seo_index: false,
    agreement_version: '1',
    agreement_at: 1,
    prices: [],
    pricing_version: 1,
    domains: [],
    visibility: {},
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>
        <SiteSettingsPanel site={site} />
      </OwnerVerificationProvider>
    </QueryClientProvider>
  )
  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(screen.queryByLabelText('Domain')).not.toBeInTheDocument()
  failed = false
  await userEvent.setup().click(retry)
  expect(await screen.findByLabelText('Domain')).toBeInTheDocument()
  client.clear()
})
