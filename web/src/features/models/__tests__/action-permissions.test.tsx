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
import {
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
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
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { DataTableBulkActions } from '../components/data-table-bulk-actions'
import { DataTableRowActions } from '../components/data-table-row-actions'
import { ModelDeleteDialog } from '../components/dialogs/model-delete-dialog'
import { PrefillGroupManagement } from '../components/dialogs/prefill-group-management'
import { SyncWizardDialog } from '../components/dialogs/sync-wizard-dialog'
import { VendorMutateDialog } from '../components/dialogs/vendor-mutate-dialog'
import { VendorOperationDialog } from '../components/dialogs/vendor-operation-dialog'
import { ModelMutateDrawer } from '../components/drawers/model-mutate-drawer'
import { PrefillGroupFormDrawer } from '../components/drawers/prefill-group-form-drawer'
import { ModelsPrimaryButtons } from '../components/models-primary-buttons'
import { ModelsProvider } from '../components/models-provider'
import type {
  MetadataSyncCandidate,
  MetadataSyncPreview,
  Model,
  PrefillGroup,
} from '../types'

type Grants = Partial<Record<'create' | 'write' | 'delete', boolean>>
type Mode =
  | 'primary'
  | 'row'
  | 'bulk'
  | 'create'
  | 'edit'
  | 'delete'
  | 'sync'
  | 'vendor-create'
  | 'vendor-edit'
  | 'assign'
  | 'merge'
  | 'vendor-delete'
  | 'prefill-create'
  | 'prefill-edit'
  | 'prefill'
const model: Model = {
  id: 42,
  model_name: 'saved-model',
  description: 'Saved description',
  status: 1,
  sync_official: 1,
  name_rule: 0,
  vendor_id: 3,
  endpoints: '',
  created_time: 1,
  updated_time: 1,
}
const vendor = {
  id: 3,
  name: 'Saved vendor',
  icon: '',
  description: 'Saved vendor description',
  status: 1,
  created_time: 1,
  updated_time: 1,
  model_count: 0,
  version: 'vendor-v1',
}
const group: PrefillGroup = {
  id: 9,
  name: 'Saved bundle',
  description: '',
  type: 'model',
  items: '["saved-model"]',
}
const source = {
  locale: 'en' as const,
  models_url: 'https://example.test/models.json',
  vendors_url: 'https://example.test/vendors.json',
  version: 'source-v1',
}
const addition: MetadataSyncCandidate = {
  model_name: 'new-model',
  kind: 'create',
  scope: 'site',
  record_version: 'new-v1',
  fields: [{ field: 'description', local: '', upstream: 'New description' }],
}
const update: MetadataSyncCandidate = {
  model_name: 'saved-model',
  kind: 'update',
  scope: 'site',
  record_version: 'saved-v1',
  vendor_to_create: 'New vendor',
  fields: [
    {
      field: 'description',
      local: 'Saved description',
      upstream: 'New description',
    },
    { field: 'vendor', local: 'Saved vendor', upstream: 'New vendor' },
  ],
}
let preview: MetadataSyncPreview
const requests: Array<{ method: string; url: string; body?: unknown }> = []
const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState().auth
const success = <T,>(data: T) => ({ data: { success: true, data } })
function setGrants(grants: Grants, channelWrite = false, root = false) {
  useAuthStore.getState().auth.setUser({
    id: 10,
    username: 'model-admin',
    role: root ? 100 : 10,
    permissions: {
      admin_permissions: {
        adty_model: { read: true, ...grants },
        channel: { read: true, write: channelWrite },
      },
    },
  })
}
function Harness({ mode }: { mode: Mode }) {
  const table = useReactTable({
    data: [model],
    columns: [{ accessorKey: 'model_name' }],
    getCoreRowModel: getCoreRowModel(),
    initialState: { rowSelection: { '0': true } },
  })
  if (mode === 'primary') return <ModelsPrimaryButtons />
  if (mode === 'row') {
    return <DataTableRowActions row={table.getRowModel().rows[0]} />
  }
  if (mode === 'bulk') return <DataTableBulkActions table={table} />
  if (mode === 'create' || mode === 'edit') {
    return (
      <ModelMutateDrawer
        open
        onOpenChange={() => {}}
        currentRow={mode === 'edit' ? model : null}
      />
    )
  }
  if (mode === 'delete') {
    return <ModelDeleteDialog models={[model]} onClose={() => {}} />
  }
  if (mode === 'sync') return <SyncWizardDialog open onOpenChange={() => {}} />
  if (mode === 'vendor-create' || mode === 'vendor-edit') {
    return (
      <VendorMutateDialog
        open
        onOpenChange={() => {}}
        currentVendor={mode === 'vendor-edit' ? vendor : null}
      />
    )
  }
  if (mode === 'assign' || mode === 'merge' || mode === 'vendor-delete') {
    return (
      <VendorOperationDialog
        selection={{
          action: mode === 'vendor-delete' ? 'delete' : mode,
          vendor_ids: [3],
          model_ids: mode === 'assign' ? [42] : undefined,
          target_vendor_id: mode === 'vendor-delete' ? undefined : 4,
        }}
        onClose={() => {}}
      />
    )
  }
  if (mode === 'prefill') {
    return <PrefillGroupManagement open onOpenChange={() => {}} />
  }
  return (
    <PrefillGroupFormDrawer
      open
      onClose={() => {}}
      currentGroup={mode === 'prefill-edit' ? group : null}
    />
  )
}
async function mount(
  mode: Mode,
  grants: Grants,
  options: { channelWrite?: boolean; root?: boolean } = {}
) {
  setGrants(grants, options.channelWrite, options.root)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const route = createRoute({
    getParentRoute: () => auth,
    path: 'models/$section',
    component: () => (
      <ModelsProvider>
        <Harness mode={mode} />
      </ModelsProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([route])]),
    history: createMemoryHistory({ initialEntries: ['/models/metadata'] }),
  })
  await router.load()
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { client, view, user: userEvent.setup() }
}
beforeEach(() => {
  requests.length = 0
  preview = { source, candidates: [addition, update] }
  onlineManager.setOnline(true)
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/models/42') return success(model)
    if (url === '/api/models/99') {
      return success({ ...model, id: 99, model_name: 'new-model' })
    }
    if (url === '/api/vendors/3') return success(vendor)
    if (url === '/api/vendors/' || url === '/api/vendors/search') {
      return success({
        items: [vendor, { ...vendor, id: 4, name: 'Target vendor' }],
        total: 2,
      })
    }
    if (url === '/api/models/search' || url === '/api/models/') {
      return success({ items: [model], total: 1 })
    }
    if (url.startsWith('/api/models/sync_upstream/preview')) {
      return success(preview)
    }
    if (url === '/api/prefill_group') return success([group])
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    requests.push({ method: 'POST', url, body })
    if (url === '/api/vendors/operations/preview') {
      return success({
        action: (body as { action: string }).action,
        sources: [vendor],
        target: { ...vendor, id: 4 },
        models: [],
        version: 'preview-v1',
      })
    }
    if (url === '/api/vendors/operations') {
      return success({ updated_models: [42], deleted_vendors: [] })
    }
    if (url === '/api/models/sync_upstream') {
      return success({
        created_models: ['new-model'],
        updated_models: [],
        created_vendors: [],
      })
    }
    return success({ ...model, id: 99, model_name: 'new-model' })
  })
  vi.spyOn(api, 'put').mockImplementation(async (url, body) => {
    requests.push({ method: 'PUT', url, body })
    return success(model)
  })
  vi.spyOn(api, 'delete').mockImplementation(async (url) => {
    requests.push({ method: 'DELETE', url })
    return success({ deleted_count: 1, updated_channels: 0 })
  })
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  onlineManager.setOnline(true)
  useAuthStore.setState({ auth: originalAuth })
  localStorage.clear()
  vi.restoreAllMocks()
})
async function queueAndRevoke(
  result: Awaited<ReturnType<typeof mount>>,
  button: HTMLElement,
  remaining: Grants,
  channelWrite = false
) {
  act(() => onlineManager.setOnline(false))
  await result.user.click(button)
  await waitFor(() =>
    expect(
      result.client
        .getMutationCache()
        .getAll()
        .some((mutation) => mutation.state.isPaused)
    ).toBe(true)
  )
  act(() => setGrants(remaining, channelWrite))
  result.view.unmount()
  await act(async () => {
    onlineManager.setOnline(true)
    await result.client.resumePausedMutations()
  })
  await waitFor(() => expect(result.client.isMutating()).toBe(0))
}

describe('independent model metadata actions', () => {
  it('disables model creation when only Write is granted', async () => {
    await mount('primary', { write: true })
    expect(screen.getByRole('button', { name: 'Add Model' })).toBeDisabled()
  })
  it('creates with Create and requires Write after the controlled drawer remains open', async () => {
    const { user } = await mount('create', { create: true })
    fireEvent.change(await screen.findByLabelText('Model Name *'), {
      target: { value: 'new-model' },
    })
    await user.click(screen.getByRole('button', { name: 'Save metadata' }))
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({ method: 'POST', url: '/api/models/' })
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save metadata' })
      ).toBeDisabled()
    )
    await user.click(screen.getByRole('button', { name: 'Save metadata' }))
    expect(requests).toHaveLength(1)
  })
  it.each([{ create: true }, { delete: true }])(
    'does not save an existing model with %j alone',
    async (grants) => {
      await mount('edit', grants)
      await screen.findByDisplayValue('Saved description')
      expect(
        screen.getByRole('button', { name: 'Save metadata' })
      ).toBeDisabled()
    }
  )
  it('saves existing metadata with Write alone', async () => {
    const { user } = await mount('edit', { write: true })
    fireEvent.change(await screen.findByDisplayValue('Saved description'), {
      target: { value: 'Updated description' },
    })
    await user.click(screen.getByRole('button', { name: 'Save metadata' }))
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({
      method: 'PUT',
      url: '/api/models/',
      body: { id: 42, description: 'Updated description' },
    })
  })
  it('blocks a queued metadata update after Write is revoked and the drawer unmounts', async () => {
    const result = await mount('edit', { write: true })
    await screen.findByDisplayValue('Saved description')
    await queueAndRevoke(
      result,
      screen.getByRole('button', { name: 'Save metadata' }),
      { create: true }
    )
    expect(requests).toEqual([])
  })
  it('disables status updates and bulk vendor assignment with Create alone', async () => {
    const row = await mount('row', { create: true })
    await row.user.click(screen.getByRole('button', { name: 'Open menu' }))
    expect(
      screen.getByRole('menuitem', { name: 'Hide from model square' })
    ).toHaveAttribute('aria-disabled', 'true')
    row.view.unmount()
    await mount('bulk', { create: true })
    expect(screen.getByRole('button', { name: 'Change vendor' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Delete selected models' })
    ).toBeDisabled()
  })
})

describe('model deletion dependencies', () => {
  it('deletes metadata with Delete alone while channel and pricing removal remain unavailable', async () => {
    const { user } = await mount('delete', { delete: true })
    expect(
      screen.getByRole('checkbox', { name: 'Also remove from all channels' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('checkbox', { name: 'Also remove pricing' })
    ).toHaveAttribute('aria-disabled', 'true')
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(requests).toEqual([{ method: 'DELETE', url: '/api/models/42' }])
    )
    expect(api.delete).toHaveBeenCalledWith('/api/models/42', {
      params: { remove_from_channels: false, remove_pricing: false },
    })
  })
  it('requires Delete independently of Write', async () => {
    await mount('delete', { write: true })
    expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  })
  it('permits channel reference removal with Model Delete and Channel Write', async () => {
    const { user } = await mount(
      'delete',
      { delete: true },
      { channelWrite: true }
    )
    await user.click(
      screen.getByRole('checkbox', { name: 'Also remove from all channels' })
    )
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(api.delete).toHaveBeenCalledWith('/api/models/42', {
        params: { remove_from_channels: true, remove_pricing: false },
      })
    )
  })
  it.each(['model-delete', 'channel-write', 'root'] as const)(
    'blocks queued deletion when selected %s authority is revoked before unmount and resume',
    async (authority) => {
      const result = await mount(
        'delete',
        { delete: true },
        { channelWrite: true, root: authority === 'root' }
      )
      if (authority !== 'model-delete') {
        await result.user.click(
          screen.getByRole('checkbox', {
            name:
              authority === 'root'
                ? 'Also remove pricing'
                : 'Also remove from all channels',
          })
        )
      }
      await queueAndRevoke(
        result,
        screen.getByRole('button', { name: 'Delete' }),
        authority === 'model-delete' ? { write: true } : { delete: true }
      )
      expect(requests).toEqual([])
    }
  )
})

describe('vendor actions', () => {
  it('creates vendor metadata with Create alone', async () => {
    const { user } = await mount('vendor-create', { create: true })
    fireEvent.change(await screen.findByLabelText('Vendor Name *'), {
      target: { value: 'New vendor' },
    })
    await user.click(screen.getByRole('button', { name: 'Save metadata' }))
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({
      method: 'POST',
      url: '/api/vendors/',
      body: { name: 'New vendor' },
    })
  })
  it('blocks a queued vendor creation after Create is revoked and the dialog unmounts', async () => {
    const result = await mount('vendor-create', { create: true })
    fireEvent.change(await screen.findByLabelText('Vendor Name *'), {
      target: { value: 'New vendor' },
    })
    await queueAndRevoke(
      result,
      screen.getByRole('button', { name: 'Save metadata' }),
      { write: true }
    )
    expect(requests).toEqual([])
  })
  it.each([
    { mode: 'vendor-create', grants: { write: true } },
    { mode: 'vendor-edit', grants: { create: true } },
  ] as const)(
    'requires the matching save action for $mode',
    async ({ mode, grants }) => {
      await mount(mode, grants)
      if (mode === 'vendor-edit') {
        await screen.findByDisplayValue('Saved vendor')
      }
      expect(
        screen.getByRole('button', { name: 'Save metadata' })
      ).toBeDisabled()
    }
  )
  it.each([
    { mode: 'assign', grants: { write: true }, allowed: true },
    { mode: 'merge', grants: { write: true }, allowed: false },
    { mode: 'merge', grants: { delete: true }, allowed: false },
    { mode: 'merge', grants: { write: true, delete: true }, allowed: true },
    { mode: 'vendor-delete', grants: { delete: true }, allowed: true },
    { mode: 'vendor-delete', grants: { write: true }, allowed: false },
  ] as const)(
    'applies $mode only with its own required actions: $grants',
    async ({ mode, grants, allowed }) => {
      const { user } = await mount(mode, grants)
      const previewButton = await screen.findByRole('button', {
        name: 'Preview changes',
      })
      await waitFor(() => expect(previewButton).toBeEnabled())
      await user.click(previewButton)
      const apply = await screen.findByRole('button', { name: 'Apply changes' })
      if (allowed) {
        await user.click(apply)
        await waitFor(() =>
          expect(
            requests.some(
              (request) => request.url === '/api/vendors/operations'
            )
          ).toBe(true)
        )
      } else {
        expect(apply).toBeDisabled()
        await user.click(apply)
        expect(requests.map((request) => request.url)).toEqual([
          '/api/vendors/operations/preview',
        ])
      }
    }
  )
  it('blocks a queued vendor merge after Delete is revoked and the dialog unmounts', async () => {
    const result = await mount('merge', { write: true, delete: true })
    const button = await screen.findByRole('button', {
      name: 'Preview changes',
    })
    await waitFor(() => expect(button).toBeEnabled())
    await result.user.click(button)
    const apply = await screen.findByRole('button', { name: 'Apply changes' })
    requests.length = 0
    await queueAndRevoke(result, apply, { write: true })
    expect(requests).toEqual([])
  })
})

async function reviewSync(
  user: ReturnType<typeof userEvent.setup>,
  names: string[],
  fields: Array<'description' | 'vendor'> = []
) {
  await user.click(
    screen.getByRole('button', { name: 'Load metadata preview' })
  )
  for (const name of names) {
    await user.click(
      await screen.findByRole('checkbox', { name: `Select ${name}` })
    )
  }
  await user.click(
    screen.getByRole('button', { name: 'Preview selected changes' })
  )
  for (const field of fields) {
    await user.click(
      screen.getByRole('checkbox', {
        name: `Apply ${field === 'vendor' ? 'Vendor' : 'Description'} for saved-model`,
      })
    )
  }
  await user.click(screen.getByRole('button', { name: 'Review confirmation' }))
  return screen.getByRole('button', {
    name: `Apply ${names.length} model changes`,
  })
}
describe('metadata sync action intersection', () => {
  it('creates selected metadata with Create alone', async () => {
    const { user } = await mount('sync', { create: true })
    await user.click(await reviewSync(user, ['new-model']))
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({
      url: '/api/models/sync_upstream',
      body: {
        selections: [
          {
            model_name: 'new-model',
            create: true,
            fields: ['description'],
            record_version: 'new-v1',
          },
        ],
      },
    })
  })
  it('updates selected existing fields with Write alone without requiring an unselected vendor creation', async () => {
    const { user } = await mount('sync', { write: true })
    await user.click(await reviewSync(user, ['saved-model'], ['description']))
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toMatchObject({
      body: {
        selections: [
          {
            model_name: 'saved-model',
            create: false,
            fields: ['description'],
            record_version: 'saved-v1',
          },
        ],
      },
    })
  })
  it.each([
    {
      grants: { create: true },
      names: ['new-model', 'saved-model'],
      fields: ['description'],
    },
    { grants: { write: true }, names: ['saved-model'], fields: ['vendor'] },
  ] as const)(
    'blocks sync missing one actual required action: $grants $names $fields',
    async ({ grants, names, fields }) => {
      const { user } = await mount('sync', grants)
      const apply = await reviewSync(user, [...names], [...fields])
      expect(apply).toBeDisabled()
      await user.click(apply)
      expect(requests).toEqual([])
    }
  )
  it.each([
    { remaining: { create: true }, action: 'Write' },
    { remaining: { write: true }, action: 'Create' },
  ] as const)(
    'blocks queued mixed sync after $action is revoked and the wizard unmounts',
    async ({ remaining }) => {
      const result = await mount('sync', { create: true, write: true })
      const apply = await reviewSync(
        result.user,
        ['new-model', 'saved-model'],
        ['description']
      )
      await queueAndRevoke(result, apply, remaining)
      expect(requests).toEqual([])
    }
  )
})

describe('prefill group actions preserve channel-consumed data', () => {
  it.each([
    {
      mode: 'prefill-create',
      grants: { create: true },
      method: 'POST',
      items: [],
    },
    {
      mode: 'prefill-edit',
      grants: { write: true },
      method: 'PUT',
      items: ['saved-model'],
    },
  ] as const)(
    'saves $mode with its own action and preserves the items payload',
    async ({ mode, grants, method, items }) => {
      const { user } = await mount(mode, grants)
      fireEvent.change(await screen.findByLabelText('Group Name'), {
        target: { value: 'Saved bundle' },
      })
      await user.click(
        screen.getByRole('button', {
          name: mode === 'prefill-create' ? 'Create' : 'Save changes',
        })
      )
      await waitFor(() => expect(requests).toHaveLength(1))
      expect(requests[0]).toMatchObject({
        method,
        url: '/api/prefill_group',
        body: { name: 'Saved bundle', type: 'model', items: [...items] },
      })
    }
  )
  it('blocks an open prefill deletion confirmation after Delete is revoked', async () => {
    const { user } = await mount('prefill', { delete: true })
    await screen.findByText('Saved bundle')
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    await user.click(screen.getByRole('menuitem', { name: 'Delete group' }))
    act(() => setGrants({ write: true }))
    const confirm = screen.getByRole('button', { name: 'Delete' })
    expect(confirm).toBeDisabled()
    await user.click(confirm)
    expect(requests).toEqual([])
  })
  it.each([
    { mode: 'prefill-create', grants: { write: true } },
    { mode: 'prefill-edit', grants: { create: true } },
  ] as const)(
    'requires the matching save action for $mode',
    async ({ mode, grants }) => {
      await mount(mode, grants)
      expect(
        screen.getByRole('button', { name: /^(Create|Update|Save)/ })
      ).toBeDisabled()
    }
  )
  it('disables editing and deletion while preserving the readable group list for Create alone', async () => {
    const { user } = await mount('prefill', { create: true })
    expect(await screen.findByText('Saved bundle')).toBeVisible()
    expect(screen.getByRole('button', { name: 'New Group' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Edit group' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    expect(
      screen.getByRole('menuitem', { name: 'Delete group' })
    ).toHaveAttribute('aria-disabled', 'true')
  })
})
