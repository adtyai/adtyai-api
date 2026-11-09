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
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { siteAdminKeys } from '../api'
import { SiteDetailSheet } from '../components/site-detail-sheet'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

function setup(role = 100) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role })
  const site = {
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
  }
  const detail = { site, domains: [], sender_domains: [], audits: [] }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    let data: unknown = detail
    if (path.endsWith('/stats')) data = { stats: { total: {} } }
    if (path.endsWith('/customers')) data = { items: [], total: 0 }
    if (path.startsWith('/api/user/search?')) {
      data = {
        items: [
          {
            id: 20,
            username: 'next-owner',
            display_name: 'New owner',
            site_id: 0,
            status: 1,
            role: 1,
          },
        ],
        total: 1,
      }
    }
    return { data: { success: true, data } }
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { ...site, owner_user_id: 20, ownership_version: 1 },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteDetailSheet siteId={11} onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  return { post, client, detail }
}

async function fillHandover() {
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'Change site owner' })
  )
  const dialog = within(
    screen.getByRole('dialog', { name: 'Change site owner' })
  )
  await user.type(
    dialog.getByRole('textbox', { name: 'Find a main-site user' }),
    'next-owner'
  )
  await user.click(dialog.getByRole('button', { name: 'Search users' }))
  await user.click(dialog.getByRole('combobox', { name: 'New site owner' }))
  await user.click(
    await screen.findByRole('option', { name: 'next-owner (#20)' })
  )
  await user.type(
    dialog.getByRole('textbox', { name: 'Handover reason' }),
    'Owner replacement'
  )
  return { user, dialog }
}

it('Root reviews and confirms the selected owner before posting the versioned handover', async () => {
  const { post, client } = setup()
  const { user, dialog } = await fillHandover()
  await user.click(dialog.getByRole('button', { name: 'Review handover' }))
  const confirmation = within(
    screen.getByRole('alertdialog', { name: 'Confirm owner handover' })
  )
  expect(confirmation.getByText(/old-owner/)).toBeInTheDocument()
  expect(confirmation.getByText(/next-owner/)).toBeInTheDocument()
  await user.click(confirmation.getByRole('button', { name: 'Cancel' }))
  expect(post).not.toHaveBeenCalled()
  expect(dialog.getByRole('textbox', { name: 'Handover reason' })).toHaveValue(
    'Owner replacement'
  )
  await user.click(dialog.getByRole('button', { name: 'Review handover' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Change site owner',
    })
  )
  await waitFor(() =>
    expect(post).toHaveBeenCalledExactlyOnceWith(
      '/api/adty/admin/sites/11/owner',
      {
        expected_owner_user_id: 10,
        expected_ownership_version: 0,
        new_owner_user_id: 20,
        reason: 'Owner replacement',
      }
    )
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Change site owner' })
    ).not.toBeInTheDocument()
  )
  client.clear()
})

it('a failed handover retains its draft and protects it when closing', async () => {
  const { post, client } = setup()
  post.mockResolvedValueOnce({
    data: {
      success: false,
      message: 'Site ownership changed; reload the site before trying again',
    },
  })
  const { user, dialog } = await fillHandover()
  await user.click(dialog.getByRole('button', { name: 'Review handover' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Change site owner',
    })
  )
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(
      screen.queryByRole('alertdialog', { name: 'Confirm owner handover' })
    ).not.toBeInTheDocument()
  )
  expect(dialog.getByRole('textbox', { name: 'Handover reason' })).toHaveValue(
    'Owner replacement'
  )
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Stay' }))
  expect(dialog.getByRole('textbox', { name: 'Handover reason' })).toHaveValue(
    'Owner replacement'
  )
  expect(post).toHaveBeenCalledTimes(1)
  client.clear()
})

it('an administrator cannot open a handover and Root revocation closes the prepared action', async () => {
  const first = setup(10)
  await screen.findByText('alpha.example')
  expect(
    screen.queryByRole('button', { name: 'Change site owner' })
  ).not.toBeInTheDocument()
  cleanup()
  first.client.clear()
  const { post, client } = setup()
  const { user, dialog } = await fillHandover()
  await user.click(dialog.getByRole('button', { name: 'Review handover' }))
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 10 })
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('alertdialog', { name: 'Confirm owner handover' })
    ).not.toBeInTheDocument()
  )
  expect(post).not.toHaveBeenCalled()
  client.clear()
})

it('a newer site version blocks a prepared handover without replacing its draft', async () => {
  const { post, client, detail } = setup()
  const { dialog } = await fillHandover()
  act(() =>
    client.setQueryData(siteAdminKeys.site(11), {
      ...detail,
      site: { ...detail.site, ownership_version: 2 },
    })
  )
  await waitFor(() =>
    expect(
      dialog.getByRole('button', { name: 'Review handover' })
    ).toBeDisabled()
  )
  expect(dialog.getByRole('textbox', { name: 'Handover reason' })).toHaveValue(
    'Owner replacement'
  )
  expect(post).not.toHaveBeenCalled()
  client.clear()
})
