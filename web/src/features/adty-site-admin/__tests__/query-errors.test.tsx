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
import { useAuthStore } from '@/stores/auth-store'

import { InviteCodesPanel } from '../components/invite-codes-panel'
import { OverviewPanel } from '../components/overview-panel'
import { ReviewsPanel } from '../components/reviews-panel'
import { SitesPanel } from '../components/sites-panel'
import { UsagePanel } from '../components/usage-panel'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it.each([
  {
    name: 'sites',
    node: <SitesPanel onOpenSite={() => {}} />,
    data: { items: [], total: 0 },
    ready: 'No data',
  },
  {
    name: 'invite codes',
    node: (
      <InviteCodesPanel canCreate={false} canWrite={false} canDelete={false} />
    ),
    data: { items: [], total: 0 },
    ready: 'No data',
  },
  {
    name: 'reviews',
    node: <ReviewsPanel />,
    data: { items: [], total: 0 },
    ready: 'No data',
  },
  {
    name: 'overview',
    node: <OverviewPanel />,
    data: {
      settlement_holds: [],
      site_revenue_usd: '0',
      site_earning_usd: '0',
      site_topup_usd: '0',
      custom_hostnames: 0,
      hostname_monthly_fee_usd: '0',
      earning_hook_failures: 0,
    },
    ready: 'Revenue',
  },
  {
    name: 'usage',
    node: <UsagePanel />,
    data: {
      sender: { in_use: 0, verified: 0, failed: 0, quota_full_at: 0 },
      widgets: [],
      widget_slots_left: 10,
      unassigned_domains: 0,
    },
    ready: 'Sender domains (Resend)',
  },
])('recovers $name from a failed GET with an explicit retry', async (entry) => {
  let failed = true
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async () => {
    if (failed) throw new Error('offline')
    return { data: { success: true, data: entry.data } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>{entry.node}</QueryClientProvider>
  )
  await screen.findByRole('button', { name: 'Retry' })
  failed = false
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))

  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  )
  expect((await screen.findAllByText(entry.ready)).length).toBeGreaterThan(0)
  client.clear()
})
