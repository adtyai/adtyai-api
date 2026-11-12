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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SiteDetailSheet } from '../components/site-detail-sheet'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

function setup(role = 100) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role })
  const detail = {
    site: {
      id: 11,
      owner_user_id: 10,
      ownership_version: 0,
      owner_username: 'old-owner',
      status: 'active',
      host: 'alpha.example',
      cost_factor: null,
      max_markup: null,
      brand: '',
      visibility: '',
      customers: 0,
      created_at: 1,
      close_execute_at: 0,
      contact_info: '',
    },
    domains: [],
    sender_domains: [],
    audits: [],
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    let data: unknown = detail
    if (path.endsWith('/stats')) data = { stats: { total: {} } }
    if (path.endsWith('/customers')) data = { items: [], total: 0 }
    return { data: { success: true, data } }
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: detail.site },
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
        })
      }
    >
      <SiteDetailSheet siteId={11} onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  return { post }
}

it('Root sees that a site owner cannot be replaced and no handover is posted', async () => {
  const { post } = setup()
  expect(
    await screen.findByText('Site owners cannot be replaced')
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Change site owner' })
  ).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled()
})

it('an administrator does not see the owner replacement notice', async () => {
  setup(10)
  await screen.findByText('alpha.example')
  expect(
    screen.queryByText('Site owners cannot be replaced')
  ).not.toBeInTheDocument()
})
