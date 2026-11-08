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
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { siteAdminKeys } from '../api'
import { SiteDetailSheet } from '../components/site-detail-sheet'
import type { AdminSite } from '../types'

function site(id = 1, visibility = ''): AdminSite {
  return {
    id,
    host: `site${id}.example`,
    subdomain: `site${id}`,
    root_domain: 'example',
    owner_user_id: id + 10,
    owner_username: 'owner',
    status: 'active',
    cost_factor: '0.5',
    max_markup: '2',
    tier_cost_factor: null,
    tier_month: '',
    visibility,
    brand: '',
    contact_info: '',
    opened_by: 'admin',
    review_note: '',
    close_requested_at: 0,
    close_execute_at: 0,
    close_transfer_to: 0,
    close_claw_back: false,
    close_reason: '',
    closed_at: 0,
    customers: 0,
    created_at: 1,
    updated_at: 1,
  }
}
const clients: QueryClient[] = []
function mount(initial = site(), actions = ['read', 'adjust']) {
  useAuthStore.getState().auth.setUser({
    id: 9,
    username: 'operator',
    role: 10,
    permissions: {
      admin_permissions: {
        adty_site: Object.fromEntries(actions.map((key) => [key, true])),
      },
    },
  })
  const saved = new Map([
    [initial.id, initial],
    [2, site(2)],
  ])
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    let data: unknown
    if (path.endsWith('/stats')) data = { stats: { total: {} } }
    else if (path.endsWith('/customers')) data = { items: [], total: 0 }
    else {
      const id = Number(path.split('/').at(-1))
      data = {
        site: saved.get(id),
        domains: [],
        sender_domains: [],
        audits: [],
      }
    }
    return { data: { success: true, data } }
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (url, body) => {
    const id = Number(String(url).split('/').at(-1))
    const current = saved.get(id)
    if (!current) throw new Error('Unknown synthetic site')
    const next = { ...current, ...(body as object) }
    saved.set(id, next)
    return { data: { success: true, data: next } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const onOpenChange = vi.fn()
  const view = render(
    <QueryClientProvider client={client}>
      <SiteDetailSheet siteId={initial.id} onOpenChange={onOpenChange} />
    </QueryClientProvider>
  )
  return { saved, put, client, view, onOpenChange, user: userEvent.setup() }
}
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it('edits all six site overrides using existing values and sends only changed settings with a reason', async () => {
  const { user, put } = mount()
  const email = await screen.findByLabelText('Customer email')
  for (const label of [
    'Customer balance',
    'Customer top-ups',
    'Customer usage',
    'Request logs',
    'Customer API keys',
  ]) {
    expect(screen.getByLabelText(label)).toHaveValue('')
  }
  expect(email).toHaveValue('')
  await user.selectOptions(email, 'full')
  await user.selectOptions(screen.getByLabelText('Customer API keys'), 'off')
  const save = screen.getByRole('button', { name: 'Save site settings' })
  expect(save).toBeDisabled()
  await user.type(screen.getByLabelText('Reason'), 'Approved owner access')
  await user.click(save)
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/admin/sites/1', {
      visibility: '{"customer_email":"full","customer_tokens":"off"}',
      reason: 'Approved owner access',
    })
  )
  await waitFor(() => expect(screen.getByLabelText('Reason')).toHaveValue(''))
  expect(save).toBeDisabled()
})

it('clears one override to inheritance and can clear the remaining overrides to the global defaults', async () => {
  const { user, put, saved } = mount(
    site(1, '{"customer_email":"full","customer_tokens":"off"}')
  )
  await user.selectOptions(await screen.findByLabelText('Customer email'), '')
  await user.type(screen.getByLabelText('Reason'), 'Restore email inheritance')
  await user.click(screen.getByRole('button', { name: 'Save site settings' }))
  await waitFor(() =>
    expect(saved.get(1)?.visibility).toBe('{"customer_tokens":"off"}')
  )
  await waitFor(() => expect(screen.getByLabelText('Reason')).toHaveValue(''))
  await user.selectOptions(screen.getByLabelText('Customer API keys'), '')
  await user.type(screen.getByLabelText('Reason'), 'Restore all inheritance')
  await user.click(screen.getByRole('button', { name: 'Save site settings' }))
  await waitFor(() =>
    expect(put).toHaveBeenLastCalledWith('/api/adty/admin/sites/1', {
      visibility: '{}',
      reason: 'Restore all inheritance',
    })
  )
})

it('retains failed visibility and reason drafts, confirms Stay, and succeeds on retry', async () => {
  const { user, put, onOpenChange } = mount()
  put.mockRejectedValueOnce(new Error('offline'))
  await user.selectOptions(
    await screen.findByLabelText('Customer balance'),
    'off'
  )
  await user.type(screen.getByLabelText('Reason'), 'Restrict balance access')
  const save = screen.getByRole('button', { name: 'Save site settings' })
  await user.click(save)
  await waitFor(() => expect(put).toHaveBeenCalledOnce())
  await waitFor(() => expect(save).toBeEnabled())
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Stay' }))
  expect(screen.getByLabelText('Customer balance')).toHaveValue('off')
  expect(screen.getByLabelText('Reason')).toHaveValue('Restrict balance access')
  expect(onOpenChange).not.toHaveBeenCalled()
  await user.click(save)
  await waitFor(() => expect(screen.getByLabelText('Reason')).toHaveValue(''))
})

it('merges refreshed pristine server overrides while preserving dirty visibility choices', async () => {
  const { user, client, saved } = mount()
  await user.selectOptions(
    await screen.findByLabelText('Customer API keys'),
    'off'
  )
  saved.set(1, site(1, '{"customer_email":"full","customer_tokens":"on"}'))
  await act(async () => {
    await client.invalidateQueries({ queryKey: siteAdminKeys.site(1) })
  })
  await waitFor(() =>
    expect(screen.getByLabelText('Customer email')).toHaveValue('full')
  )
  expect(screen.getByLabelText('Customer API keys')).toHaveValue('off')
})

it('does not carry one site visibility draft into another site', async () => {
  const { user, view, client, onOpenChange } = mount()
  await user.selectOptions(
    await screen.findByLabelText('Customer API keys'),
    'off'
  )
  view.rerender(
    <QueryClientProvider client={client}>
      <SiteDetailSheet siteId={2} onOpenChange={onOpenChange} />
    </QueryClientProvider>
  )
  await screen.findByText('site2.example')
  expect(screen.getByLabelText('Customer API keys')).toHaveValue('')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  expect(screen.queryByRole('button', { name: 'Stay' })).not.toBeInTheDocument()
})

it('keeps site visibility read-only without the site adjust permission', async () => {
  const { put } = mount(site(), ['read', 'review'])
  expect(await screen.findByLabelText('Customer email')).toBeDisabled()
  expect(screen.getByLabelText('Customer API keys')).toBeDisabled()
  expect(
    screen.queryByRole('button', { name: 'Save site settings' })
  ).not.toBeInTheDocument()
  expect(put).not.toHaveBeenCalled()
})
it.each(['broken json', '{"future_key":"on"}', '{"customer_email":"on"}'])(
  'shows invalid saved visibility %s instead of silently replacing it',
  async (visibility) => {
    const { put, user } = mount(site(1, visibility))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Saved site visibility settings are invalid.'
    )
    expect(screen.getByLabelText('Customer email')).toBeDisabled()
    await user.type(screen.getByLabelText('Reason'), 'Trying to save')
    expect(
      screen.getByRole('button', { name: 'Save site settings' })
    ).toBeDisabled()
    expect(put).not.toHaveBeenCalled()
  }
)

it('saves visibility together with changed numeric fields and explicit numeric inheritance', async () => {
  const { put, user } = mount()
  await user.selectOptions(
    await screen.findByLabelText('Customer usage'),
    'off'
  )
  await user.clear(screen.getByLabelText('Cost factor'))
  await user.clear(screen.getByLabelText('Markup limit'))
  await user.type(screen.getByLabelText('Markup limit'), '1.5')
  await user.type(
    screen.getByLabelText('Reason'),
    'Combined site policy change'
  )
  await user.click(screen.getByRole('button', { name: 'Save site settings' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/admin/sites/1', {
      max_markup: '1.5',
      clear: ['cost_factor'],
      visibility: '{"customer_usage":"off"}',
      reason: 'Combined site policy change',
    })
  )
  await waitFor(() => expect(screen.getByLabelText('Reason')).toHaveValue(''))
})
