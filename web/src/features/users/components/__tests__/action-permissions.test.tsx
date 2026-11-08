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
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersPrimaryButtons } from '../users-primary-buttons'
import { UsersProvider } from '../users-provider'

const target: User = {
  id: 2,
  username: 'existing-user',
  display_name: 'Existing user',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
}

function setViewer(actions: string[], role = 10) {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'operator',
    role,
    permissions: {
      admin_permissions: {
        adty_user: Object.fromEntries(actions.map((action) => [action, true])),
      },
    },
  })
}

function mount(drawer?: 'create' | 'update') {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/authz/catalog') {
      return { data: { success: true, data: { resources: [], roles: [] } } }
    }
    if (url === '/api/user/2') return { data: { success: true, data: target } }
    if (url === '/api/verify/methods') {
      return {
        data: {
          success: true,
          data: {
            scope: 'admin.user.update',
            methods: [{ method: '2fa', available: true }],
            oauth_providers: [],
            password_encryption_enabled: false,
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        {drawer ? (
          <UsersMutateDrawer
            open
            onOpenChange={() => undefined}
            currentRow={drawer === 'update' ? target : undefined}
          />
        ) : (
          <UsersPrimaryButtons />
        )}
      </UsersProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
})

async function submitUserForm() {
  const form = screen.getByLabelText('Username').closest('form')
  if (!form) throw new Error('User form is missing')
  await act(async () => {
    fireEvent.submit(form)
  })
}

it.each(['read', 'write', 'delete'])(
  'does not offer creation for a user %s grant',
  (action) => {
    setViewer([action])
    mount()
    expect(
      screen.queryByRole('button', { name: 'Add User' })
    ).not.toBeInTheDocument()
  }
)

it.each([10, 100])(
  'offers creation for the exact action or Root role %s',
  (role) => {
    setViewer(role === 100 ? [] : ['create'], role)
    mount()
    expect(screen.getByRole('button', { name: 'Add User' })).toBeEnabled()
  }
)

it('does not offer creation to an ordinary user carrying a forged admin grant', () => {
  setViewer(['create'], 1)
  mount()
  expect(
    screen.queryByRole('button', { name: 'Add User' })
  ).not.toBeInTheDocument()
})

it('allows create without write and posts the ordinary user form', async () => {
  setViewer(['create'])
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  mount('create')
  await userEvent.type(screen.getByLabelText('Username'), 'new-user')
  await userEvent.type(
    screen.getByLabelText('Password'),
    'Synthetic-create#2026'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({ username: 'new-user', role: 1 }),
      {}
    )
  )
})

it('blocks a directly opened create form when only write is granted', async () => {
  setViewer(['write'])
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  mount('create')
  await userEvent.type(screen.getByLabelText('Username'), 'new-user')
  await userEvent.type(
    screen.getByLabelText('Password'),
    'Synthetic-create#2026'
  )
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  await submitUserForm()
  expect(post).not.toHaveBeenCalled()
})

it('does not let create replace write in an existing user form', async () => {
  setViewer(['create'])
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  mount('update')
  await screen.findByDisplayValue('Existing user')
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  await submitUserForm()
  expect(put).not.toHaveBeenCalled()
})

it('disables an open create form immediately after the exact grant is revoked', async () => {
  setViewer(['create'])
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  mount('create')
  await userEvent.type(screen.getByLabelText('Username'), 'new-user')
  await userEvent.type(
    screen.getByLabelText('Password'),
    'Synthetic-create#2026'
  )
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
  act(() => setViewer(['write']))
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  await submitUserForm()
  expect(post).not.toHaveBeenCalled()
})

it.each([false, true])(
  'blocks a validated create after Create revocation, unmount=%s',
  async (unmount) => {
    setViewer(['create'])
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })
    const view = mount('create')
    await userEvent.type(screen.getByLabelText('Username'), 'queued-user')
    await userEvent.type(
      screen.getByLabelText('Password'),
      'Synthetic-create#2026'
    )
    const form = screen.getByLabelText('Username').closest('form')
    if (!form) throw new Error('User form is missing')
    await act(async () => {
      fireEvent.submit(form)
      if (unmount) view.unmount()
      setViewer(['read'])
      await Promise.resolve()
    })
    expect(post).not.toHaveBeenCalled()
  }
)

it.each([false, true])(
  'blocks update after a pending proof returns following Write revocation, unmount=%s',
  async (unmount) => {
    setViewer(['write'])
    const proof = {
      data: {
        success: true,
        data: {
          proof_token: 'delayed-proof',
          method: '2fa',
          scope: 'admin.user.update',
          expires_at: Math.floor(Date.now() / 1000) + 60,
        },
      },
    }
    let release = (_value: typeof proof) => {}
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url !== '/api/verify') throw new Error(`Unexpected POST ${url}`)
      return new Promise<typeof proof>((resolve) => {
        release = resolve
      })
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const view = mount('update')
    await screen.findByDisplayValue('Existing user')
    await userEvent.type(
      screen.getByLabelText('Password'),
      'Synthetic-update#2026'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await userEvent.type(
      await screen.findByLabelText('Authenticator code or backup code'),
      '123456'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    act(() => {
      if (unmount) view.unmount()
      setViewer(['read'])
    })
    await act(async () => {
      release(proof)
    })
    expect(put).not.toHaveBeenCalled()
  }
)
