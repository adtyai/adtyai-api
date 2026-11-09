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

import { AnnouncementsSection } from '@/features/system-settings/content/announcements-section'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { NotificationManager } from '../components/notification-manager'

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})
function mount() {
  return render(
    <QueryClientProvider client={client}>
      <AnnouncementsSection enabled={false} data='[]' />
    </QueryClientProvider>
  )
}

it('publishes a notification with the selected recipient scope through the real API', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { id: 1 } } })
  mount()
  await user.click(screen.getByRole('button', { name: 'New notification' }))
  const dialog = screen.getByRole('dialog', { name: 'New notification' })
  await user.type(within(dialog).getByLabelText('Title'), 'Maintenance window')
  await user.type(
    within(dialog).getByLabelText('Content'),
    'Service resumes soon'
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Publish notification' })
  )
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/admin/notifications',
      expect.objectContaining({
        title: 'Maintenance window',
        content: 'Service resumes soon',
        audience: 'all_users',
        target_site_id: 0,
      })
    )
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'New notification' })
    ).not.toBeInTheDocument()
  )
})

it('keeps an unsuccessful publish draft and retries without reporting success', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValueOnce({
      data: { success: false, message: 'Publishing unavailable' },
    })
    .mockResolvedValueOnce({ data: { success: true, data: { id: 1 } } })
  mount()
  await user.click(screen.getByRole('button', { name: 'New notification' }))
  const dialog = screen.getByRole('dialog', { name: 'New notification' })
  await user.type(within(dialog).getByLabelText('Title'), 'Keep draft')
  await user.type(within(dialog).getByLabelText('Content'), 'Draft body')
  await user.click(
    within(dialog).getByRole('button', { name: 'Publish notification' })
  )
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(screen.getByLabelText('Title')).toHaveValue('Keep draft')
  await user.click(
    within(dialog).getByRole('button', { name: 'Publish notification' })
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(post).toHaveBeenCalledTimes(2)
})

it('requires a selected site for a targeted publish and sends the selected notification type', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/user/sites'
          ? [{ site_id: 7, site_name: 'Shop Seven' }]
          : { items: [], total: 0 },
    },
  }))
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { id: 1 } } })
  mount()
  await user.click(screen.getByRole('button', { name: 'New notification' }))
  const dialog = screen.getByRole('dialog', { name: 'New notification' })
  await user.type(within(dialog).getByLabelText('Title'), 'Only shop seven')
  await user.type(
    within(dialog).getByLabelText('Content'),
    'Maintenance details'
  )
  await user.selectOptions(
    within(dialog).getByLabelText('Notification type'),
    'maintenance'
  )
  await user.selectOptions(within(dialog).getByLabelText('Recipients'), 'site')
  await user.click(
    within(dialog).getByRole('button', { name: 'Publish notification' })
  )
  expect(
    await within(dialog).findByText('Select a site', {
      selector: '[role=alert]',
    })
  ).toBeVisible()
  expect(post).not.toHaveBeenCalled()
  await user.selectOptions(within(dialog).getByLabelText('Site'), '7')
  await user.click(
    within(dialog).getByRole('button', { name: 'Publish notification' })
  )
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/admin/notifications',
      expect.objectContaining({
        audience: 'site',
        target_site_id: 7,
        kind: 'maintenance',
      })
    )
  )
})

it('restricts the owner editor to its own site and protects an unsaved draft when closing', async () => {
  const user = userEvent.setup()
  useAuthStore.getState().auth.setUser({ id: 10, username: 'owner', role: 1 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { id: 1 } } })
  render(
    <QueryClientProvider client={client}>
      <NotificationManager mode='owner' siteId={7} />
    </QueryClientProvider>
  )
  await user.click(screen.getByRole('button', { name: 'New notification' }))
  expect(screen.getByLabelText('Recipients')).toBeDisabled()
  expect(
    screen.queryByRole('option', { name: 'All users' })
  ).not.toBeInTheDocument()
  await user.type(screen.getByLabelText('Title'), 'Site rules')
  await user.type(screen.getByLabelText('Content'), 'Customer announcement')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Stay',
    })
  )
  expect(screen.getByLabelText('Title')).toHaveValue('Site rules')
  await user.click(screen.getByRole('button', { name: 'Publish notification' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/site/notifications',
      expect.objectContaining({ target_site_id: 7, audience: 'site' })
    )
  )
})

it('sends the edited revision and retains the draft when a stale edit conflicts', async () => {
  const user = userEvent.setup()
  const row = {
    id: 9,
    title: 'Existing notice',
    content: 'Original body',
    kind: 'system',
    audience: 'all_users',
    target_site_id: 0,
    business: '',
    revision: 5,
    publisher_name: 'Root author',
    published_at: 1,
    status: 'published',
  }
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [row], total: 1 } },
  })
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: {
      success: false,
      message: 'This notification was changed elsewhere. Reload and try again',
    },
  })
  mount()
  const table = await screen.findByRole('table', {
    name: 'Published notifications',
  })
  await user.click(await within(table).findByRole('button', { name: 'Edit' }))
  await user.clear(screen.getByLabelText('Content'))
  await user.type(screen.getByLabelText('Content'), 'Changed body')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/adty/admin/notifications/9',
      expect.objectContaining({ revision: 5, content: 'Changed body' })
    )
  )
  expect(
    screen.getByRole('dialog', { name: 'Edit notification' })
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Content')).toHaveValue('Changed body')
})

it('confirms withdrawal, retries a failed request and retains the withdrawn history', async () => {
  const user = userEvent.setup()
  let status = 'published'
  const row = {
    id: 9,
    title: 'Existing notice',
    content: 'Body',
    kind: 'system',
    audience: 'all_users',
    target_site_id: 0,
    business: '',
    revision: 5,
    publisher_name: 'Root author',
    published_at: 1,
  }
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: { items: [{ ...row, status }], total: 1 } },
  }))
  const remove = vi
    .spyOn(api, 'delete')
    .mockRejectedValueOnce(new Error('Offline'))
    .mockImplementation(async () => {
      status = 'withdrawn'
      return { data: { success: true } }
    })
  mount()
  const table = await screen.findByRole('table', {
    name: 'Published notifications',
  })
  await user.click(
    await within(table).findByRole('button', { name: 'Withdraw notification' })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  await waitFor(() => expect(remove).toHaveBeenCalledTimes(1))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  await within(table).findByText('Withdrawn')
  expect(
    within(table).queryByRole('button', { name: 'Edit' })
  ).not.toBeInTheDocument()
  expect(remove).toHaveBeenLastCalledWith('/api/adty/admin/notifications/9', {
    data: { revision: 5 },
  })
})

it('offers retry after management loading fails and hides publishing from an ordinary admin', async () => {
  const user = userEvent.setup()
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('Offline'))
    .mockResolvedValue({
      data: { success: true, data: { items: [], total: 0 } },
    })
  const panel = mount()
  await user.click(await screen.findByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  panel.unmount()
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  mount()
  expect(
    screen.queryByRole('button', { name: 'New notification' })
  ).not.toBeInTheDocument()
})
