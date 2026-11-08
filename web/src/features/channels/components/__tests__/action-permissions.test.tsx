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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema } from '../../types'
import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider, useChannels } from '../channels-provider'
import { DataTableBulkActions } from '../data-table-bulk-actions'
import { DataTableRowActions } from '../data-table-row-actions'
import { CopyChannelDialog } from '../dialogs/copy-channel-dialog'
import { MultiKeyManageDialog } from '../dialogs/multi-key-manage-dialog'
import { ChannelMutateDrawer } from '../drawers/channel-mutate-drawer'

type Grants = Partial<
  Record<'create' | 'write' | 'delete' | 'operate' | 'sensitive_write', boolean>
>
type Mode = 'primary' | 'row' | 'bulk' | 'create' | 'edit' | 'keys'
type Request = { method: string; url: string; body?: unknown }
const requests: Request[] = []
const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState().auth
const channel = channelSchema.parse({
  id: 42,
  name: 'Existing channel',
  type: 1,
  key: '',
  status: 1,
  created_time: 1,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  models: 'gpt-4o',
  group: 'default',
  base_url: 'https://saved.example',
})
const response = <T,>(data: T) => ({ data: { success: true, data } })

function setGrants(grants: Grants) {
  useAuthStore.getState().auth.setUser({
    id: 10,
    username: 'channel-admin',
    role: 10,
    permissions: {
      admin_permissions: { channel: { read: true, ...grants } },
    },
  })
}

function ActionHarness(props: { mode: Mode }) {
  const { open, setOpen, currentRow, setCurrentRow } = useChannels()
  const [drawerOpen, setDrawerOpen] = useState(true)
  const table = useReactTable({
    data: [channel, { ...channel, id: 43 }],
    columns: [{ accessorKey: 'name' }],
    getCoreRowModel: getCoreRowModel(),
    initialState: { rowSelection: { '0': true, '1': true } },
  })
  useEffect(() => {
    if (props.mode === 'keys') setCurrentRow(channel)
  }, [props.mode, setCurrentRow])
  if (props.mode === 'edit' || props.mode === 'create') {
    return (
      <ChannelMutateDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        currentRow={props.mode === 'edit' ? channel : null}
      />
    )
  }
  if (props.mode === 'keys') {
    return <MultiKeyManageDialog open onOpenChange={() => {}} />
  }
  return (
    <>
      {props.mode === 'primary' && <ChannelsPrimaryButtons />}
      {props.mode === 'row' && (
        <DataTableRowActions row={table.getRowModel().rows[0]} />
      )}
      {props.mode === 'bulk' && <DataTableBulkActions table={table} />}
      {(open === 'create-channel' || open === 'update-channel') && (
        <ChannelMutateDrawer
          open
          onOpenChange={(value) => !value && setOpen(null)}
          currentRow={open === 'update-channel' ? currentRow : null}
        />
      )}
      {open === 'copy-channel' && (
        <CopyChannelDialog
          open
          onOpenChange={(value) => !value && setOpen(null)}
        />
      )}
    </>
  )
}

async function renderActions(
  mode: Mode,
  grants: Grants,
  onMutate?: () => Promise<void>
) {
  setGrants(grants)
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false, onMutate },
    },
  })
  clients.push(client)
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const route = createRoute({
    getParentRoute: () => auth,
    path: 'channels/',
    component: () => (
      <ChannelsProvider>
        <ActionHarness mode={mode} />
      </ChannelsProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([route])]),
    history: createMemoryHistory({ initialEntries: ['/channels/'] }),
  })
  await router.load()
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { user: userEvent.setup(), view, client }
}

beforeEach(() => {
  requests.length = 0
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/42') return response(channel)
    if (url === '/api/task_plugin_options') return response([])
    if (url === '/api/channel/models') {
      return response([{ id: 'gpt-4o' }, { id: 'deepseek-chat' }])
    }
    if (url === '/api/channel/default_base_urls') return response({})
    if (url === '/api/group/') return response(['default'])
    if (url === '/api/prefill_group') return response([])
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    requests.push({ method: 'POST', url, body })
    if (
      url === '/api/channel/multi_key/manage' &&
      (body as { action: string }).action === 'get_key_status'
    ) {
      return response({
        keys: [{ index: 0, status: 1, key_preview: 'key-prefix' }],
        total: 1,
        page: 1,
        page_size: 10,
        total_pages: 1,
        enabled_count: 1,
        manual_disabled_count: 0,
        auto_disabled_count: 0,
      })
    }
    if (url === '/api/channel/fetch_models') return response(['deepseek-chat'])
    return response({ id: 99 })
  })
  vi.spyOn(api, 'put').mockImplementation(async (url, body) => {
    requests.push({ method: 'PUT', url, body })
    return response(channel)
  })
  vi.spyOn(api, 'delete').mockImplementation(async (url) => {
    requests.push({ method: 'DELETE', url })
    return response(1)
  })
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState({ auth: originalAuth })
  localStorage.clear()
  vi.restoreAllMocks()
})

async function chooseProvider(user: ReturnType<typeof userEvent.setup>) {
  const provider = await screen.findByRole('option', { name: /^DeepSeek / })
  expect(provider).not.toHaveAttribute('aria-disabled', 'true')
  await user.click(provider)
  fireEvent.change(screen.getByLabelText('API Key *'), {
    target: { value: 'new-channel-secret' },
  })
  await user.click(screen.getByRole('button', { name: 'Fill Related Models' }))
}

describe('channel creation permissions', () => {
  it('creates with Create alone through provider selection without SensitiveWrite', async () => {
    const { user } = await renderActions('primary', { create: true })
    const create = screen.getByRole('button', { name: /Create Channel/ })
    expect(create).toBeEnabled()
    await user.click(create)
    await chooseProvider(user)
    await user.click(screen.getByRole('button', { name: 'Create Channel' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      url: '/api/channel',
      body: expect.objectContaining({
        mode: 'single',
        channel: expect.objectContaining({
          type: 43,
          key: 'new-channel-secret',
        }),
      }),
    })
  })

  it('does not grant channel creation through SensitiveWrite alone', async () => {
    const { user } = await renderActions('primary', { sensitive_write: true })
    const create = screen.getByRole('button', { name: /Create Channel/ })
    await user.click(create)

    expect(create).toBeDisabled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(requests).toEqual([])
  })

  it('blocks an open creation form after Create is revoked', async () => {
    const { user } = await renderActions('create', {
      create: true,
      sensitive_write: true,
    })
    await chooseProvider(user)
    act(() => setGrants({ sensitive_write: true }))
    const save = screen.getByRole('button', { name: 'Create Channel' })
    await user.click(save)

    expect(save).toBeDisabled()
    expect(requests).toEqual([])
  })

  it('keeps temporary-credential model preview unavailable with Create alone', async () => {
    const { user } = await renderActions('create', { create: true })
    await chooseProvider(user)
    const fetch = screen.getByRole('button', { name: 'Fetch from Upstream' })
    await user.click(fetch)

    expect(fetch).toBeDisabled()
    expect(requests).toEqual([])
  })
})

describe('copying channels', () => {
  it('copies with Create alone without SensitiveWrite', async () => {
    const { user } = await renderActions('row', { create: true })
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    await user.click(screen.getByRole('menuitem', { name: 'Copy Channel' }))
    const dialog = await screen.findByRole('dialog', { name: 'Copy Channel' })
    await user.click(
      within(dialog).getByRole('button', { name: 'Copy Channel' })
    )

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      url: '/api/channel/copy/42',
      body: null,
    })
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/copy/42',
      null,
      expect.objectContaining({
        params: { suffix: '_copy', reset_balance: true },
      })
    )
  })

  it('does not grant copying through SensitiveWrite alone', async () => {
    const { user } = await renderActions('row', { sensitive_write: true })
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    const copy = screen.getByRole('menuitem', { name: 'Copy Channel' })
    await user.click(copy)

    expect(copy).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.queryByRole('dialog', { name: 'Copy Channel' })
    ).not.toBeInTheDocument()
    expect(requests).toEqual([])
  })

  it('blocks an open copy dialog after Create is revoked', async () => {
    const { user } = await renderActions('row', {
      create: true,
      sensitive_write: true,
    })
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    await user.click(screen.getByRole('menuitem', { name: 'Copy Channel' }))
    const dialog = await screen.findByRole('dialog', { name: 'Copy Channel' })
    act(() => setGrants({ sensitive_write: true }))
    const copy = within(dialog).getByRole('button', { name: 'Copy Channel' })
    await user.click(copy)

    expect(copy).toBeDisabled()
    expect(requests).toEqual([])
  })
})

const deletions = [
  {
    mode: 'row',
    dialogRole: 'alertdialog',
    title: 'Delete Channel',
    confirm: 'Delete',
    method: 'DELETE',
    url: '/api/channel/42',
    body: undefined,
  },
  {
    mode: 'bulk',
    dialogRole: 'dialog',
    title: 'Delete Channels?',
    confirm: 'Delete',
    method: 'POST',
    url: '/api/channel/batch',
    body: { ids: [42, 43] },
  },
  {
    mode: 'primary',
    dialogRole: 'alertdialog',
    title: 'Delete All Disabled Channels?',
    confirm: 'Continue',
    method: 'DELETE',
    url: '/api/channel/disabled',
    body: undefined,
  },
] as const

async function openDeletion(
  user: ReturnType<typeof userEvent.setup>,
  mode: (typeof deletions)[number]['mode']
) {
  if (mode === 'row') {
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    return screen.getByRole('menuitem', { name: 'Delete' })
  }
  if (mode === 'primary') {
    await user.click(screen.getByRole('button', { name: 'More actions' }))
    return screen.getByRole('menuitem', { name: 'Delete All Disabled' })
  }
  return screen.getByRole('button', { name: 'Delete selected channels' })
}

describe.each(deletions)('$mode channel deletion', (kind) => {
  it('deletes with Delete alone without SensitiveWrite', async () => {
    const { user } = await renderActions(kind.mode, { delete: true })
    await user.click(await openDeletion(user, kind.mode))
    const dialog = await screen.findByRole(kind.dialogRole, {
      name: kind.title,
    })
    await user.click(within(dialog).getByRole('button', { name: kind.confirm }))

    await waitFor(() =>
      expect(requests).toEqual([
        { method: kind.method, url: kind.url, body: kind.body },
      ])
    )
  })

  it('does not grant channel deletion through SensitiveWrite alone', async () => {
    const { user } = await renderActions(kind.mode, { sensitive_write: true })
    await user.click(await openDeletion(user, kind.mode))

    expect(
      screen.queryByRole(kind.dialogRole, { name: kind.title })
    ).not.toBeInTheDocument()
    expect(requests).toEqual([])
  })

  it('blocks an open confirmation after Delete is revoked', async () => {
    const { user } = await renderActions(kind.mode, {
      delete: true,
      sensitive_write: true,
    })
    await user.click(await openDeletion(user, kind.mode))
    const dialog = await screen.findByRole(kind.dialogRole, {
      name: kind.title,
    })
    act(() => setGrants({ sensitive_write: true }))
    const confirm = within(dialog).getByRole('button', { name: kind.confirm })
    await user.click(confirm)

    expect(confirm).toBeDisabled()
    expect(requests).toEqual([])
  })
})

describe('saving channel edits', () => {
  it('updates routing with Write alone and omits sensitive fields', async () => {
    const { user } = await renderActions('edit', { write: true })
    const name = await screen.findByDisplayValue('Existing channel')
    fireEvent.change(name, { target: { value: 'Updated channel' } })
    await user.click(screen.getByRole('button', { name: 'Update Channel' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({
      method: 'PUT',
      url: '/api/channel/',
      body: { id: 42, name: 'Updated channel' },
    })
    expect(requests[0].body).not.toHaveProperty('key')
    expect(requests[0].body).not.toHaveProperty('setting')
  })

  it.each([{ create: true }, { sensitive_write: true }])(
    'does not update existing channels without Write (%j)',
    async (grants) => {
      const { user } = await renderActions('edit', grants)
      await screen.findByDisplayValue('Existing channel')
      const update = screen.getByRole('button', { name: 'Update Channel' })
      await user.click(update)

      expect(update).toBeDisabled()
      expect(requests).toEqual([])
    }
  )

  it('updates a key with Write and SensitiveWrite', async () => {
    const { user } = await renderActions('edit', {
      write: true,
      sensitive_write: true,
    })
    await screen.findByDisplayValue('Existing channel')
    fireEvent.change(screen.getByLabelText('API Key *'), {
      target: { value: 'replacement-key' },
    })
    await user.click(screen.getByRole('button', { name: 'Update Channel' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({
      method: 'PUT',
      url: '/api/channel/',
      body: { id: 42, key: 'replacement-key' },
    })
  })

  it('blocks an open edit after Write is revoked', async () => {
    const { user } = await renderActions('edit', {
      write: true,
      sensitive_write: true,
    })
    await screen.findByDisplayValue('Existing channel')
    act(() => setGrants({ sensitive_write: true }))
    const update = screen.getByRole('button', { name: 'Update Channel' })
    await user.click(update)

    expect(update).toBeDisabled()
    expect(requests).toEqual([])
  })

  it.each([
    { action: 'write', remaining: { sensitive_write: true }, sensitive: false },
    { action: 'sensitive_write', remaining: { write: true }, sensitive: true },
  ] as const)(
    'does not dispatch a queued save after $action is revoked and the editor unmounts',
    async (kind) => {
      let resume = () => {}
      let paused = false
      const gate = new Promise<void>((resolve) => {
        resume = resolve
      })
      const { user, view, client } = await renderActions(
        'edit',
        { write: true, sensitive_write: true },
        async () => {
          paused = true
          await gate
        }
      )
      await screen.findByDisplayValue('Existing channel')
      if (kind.sensitive) {
        fireEvent.change(screen.getByLabelText('API Key *'), {
          target: { value: 'replacement-key' },
        })
      }
      await user.click(screen.getByRole('button', { name: 'Update Channel' }))
      await waitFor(() => expect(paused).toBe(true))
      act(() => setGrants(kind.remaining))
      view.unmount()
      await act(async () => {
        resume()
      })
      await waitFor(() => expect(client.isMutating()).toBe(0))

      expect(requests).toEqual([])
    }
  )
})

describe('multi-key deletion remains sensitive', () => {
  it('does not grant key deletion through channel Delete and Operate', async () => {
    const { user } = await renderActions('keys', {
      delete: true,
      operate: true,
    })
    const remove = await screen.findByRole('button', { name: 'Delete' })
    await user.click(remove)

    expect(remove).toBeDisabled()
    expect(requests).toEqual([
      {
        method: 'POST',
        url: '/api/channel/multi_key/manage',
        body: {
          channel_id: 42,
          action: 'get_key_status',
          page: 1,
          page_size: 10,
          status: undefined,
        },
      },
    ])
  })
})
