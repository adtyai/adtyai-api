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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SiteTierControl } from '../components/site-tier-control'
import { SiteTiersPanel } from '../components/site-tiers-panel'
import type { AdminSite } from '../types'

const tiers = [
  {
    id: 1,
    code: 'gold',
    name: 'Gold',
    min_effective_users: 10,
    min_principal_usd: '100',
    min_consumption_usd: '50',
    cost_factor: '0.7',
    max_markup: '2.5',
    deposit_usd: '30',
    enabled: true,
    sort_order: 1,
  },
  {
    id: 2,
    code: 'retired',
    name: 'Retired',
    min_effective_users: 0,
    min_principal_usd: '0',
    min_consumption_usd: '0',
    cost_factor: null,
    max_markup: null,
    deposit_usd: '0',
    enabled: false,
    sort_order: 0,
  },
]

function wrap(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: tiers },
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it('is hidden from admins who are not Root', () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  wrap(<SiteTiersPanel />)
  expect(screen.queryByText('Site levels')).not.toBeInTheDocument()
})

it('lists levels and adds a new one by code', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: tiers } })
  wrap(<SiteTiersPanel />)
  const table = await screen.findByRole('table', { name: 'Site levels' })
  expect(table).toHaveTextContent('Gold')
  expect(table).toHaveTextContent('Disabled')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Add level' }))
  const dialog = await screen.findByRole('dialog', { name: 'Add level' })
  const save = within(dialog).getByRole('button', { name: 'Save' })
  expect(save).toBeDisabled()
  await user.type(within(dialog).getByLabelText('Level code'), 'silver')
  await user.type(within(dialog).getByLabelText('Level name'), 'Silver')
  await user.clear(within(dialog).getByLabelText('Minimum effective customers'))
  await user.type(
    within(dialog).getByLabelText('Minimum effective customers'),
    '5'
  )
  await user.click(save)
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/admin/reseller/tiers', {
      tiers: [
        {
          code: 'silver',
          name: 'Silver',
          min_effective_users: 5,
          min_principal_usd: '0',
          min_consumption_usd: '0',
          cost_factor: null,
          max_markup: null,
          deposit_usd: '0',
          sort_order: 0,
          enabled: true,
        },
      ],
    })
  )
})

it('keeps the code read-only when editing an existing level', async () => {
  wrap(<SiteTiersPanel />)
  const table = await screen.findByRole('table', { name: 'Site levels' })
  const row = within(table).getByRole('row', { name: /Gold/ })
  await userEvent
    .setup()
    .click(within(row).getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog', { name: 'Edit level' })
  expect(within(dialog).getByLabelText('Level code')).toHaveAttribute(
    'readonly'
  )
  expect(within(dialog).getByLabelText('Level code')).toHaveValue('gold')
})

it('pins a site to an enabled level and offers disabled levels nowhere', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: {} } })
  const site = { id: 7, tier_code: '', tier_mode: 'auto' } as AdminSite
  wrap(<SiteTierControl site={site} />)
  const select = await screen.findByLabelText('Level assignment')
  await waitFor(() =>
    expect(within(select).getAllByRole('option')).toHaveLength(2)
  )
  expect(within(select).queryByText('Pin to Retired')).not.toBeInTheDocument()
  const user = userEvent.setup()
  await user.selectOptions(select, 'gold')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/admin/sites/7/tier', {
      code: 'gold',
      mode: 'manual',
    })
  )
})
