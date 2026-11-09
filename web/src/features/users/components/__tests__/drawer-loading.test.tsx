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
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it('keeps the newly selected user when the previous user request finishes later', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'operator', role: 100 })
  const alice = userSchema.parse({
    id: 7,
    username: 'alice',
    display_name: 'Alice',
    role: 1,
    status: 1,
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    site_id: 7,
    site_name: 'North Studio',
  })
  const bob = userSchema.parse({
    ...alice,
    id: 8,
    username: 'bob',
    display_name: 'Bob',
    site_id: 8,
    site_name: 'South Studio',
  })
  let finishAlice!: (value: unknown) => void
  let finishBob!: (value: unknown) => void
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/7') {
      return await new Promise((resolve) => {
        finishAlice = resolve
      })
    }
    if (url === '/api/user/8') {
      return await new Promise((resolve) => {
        finishBob = resolve
      })
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return { data: { success: true, data: { resources: [], roles: [] } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const wrapper = (row: typeof alice) => (
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer
          open
          currentRow={row}
          onOpenChange={() => undefined}
        />
      </UsersProvider>
    </QueryClientProvider>
  )
  const view = render(wrapper(alice))
  view.rerender(wrapper(bob))
  await act(async () => {
    finishBob({
      data: {
        success: true,
        data: { ...bob, site_id: 9, site_name: 'Current Studio' },
      },
    })
  })
  expect(screen.getByLabelText('Username')).toHaveValue('bob')
  await act(async () => {
    finishAlice({ data: { success: true, data: alice } })
  })
  expect(screen.getByLabelText('Username')).toHaveValue('bob')
  expect(screen.getByLabelText('Display Name')).toHaveValue('Bob')
  expect(screen.getByLabelText('Site')).toHaveTextContent('Current Studio')
  expect(screen.getByLabelText('Site ID')).toHaveTextContent('9')
  expect(screen.queryByText('North Studio')).not.toBeInTheDocument()
  client.clear()
})
