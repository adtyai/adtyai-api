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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'operator', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return { data: { success: true, data: { resources: [], roles: [] } } }
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

function renderCreateDrawer() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer open onOpenChange={() => undefined} />
      </UsersProvider>
    </QueryClientProvider>
  )
}

async function submitNewUser(username: string) {
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Username'), username)
  await user.type(screen.getByLabelText(/^Password/), 'password-123')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
}

it.each([
  ['contains @', 'victim@example.com', 'Username cannot contain @'],
  [
    'is longer than 20 characters',
    'a'.repeat(21),
    'Username must be 20 characters or fewer',
  ],
])(
  'admin user form shows an error and sends nothing when the username %s',
  async (_case, username, message) => {
    const post = vi.spyOn(api, 'post')
    renderCreateDrawer()

    await submitNewUser(username)

    expect(await screen.findByText(message)).toBeInTheDocument()
    expect(post).not.toHaveBeenCalled()
  }
)

it('admin user form creates a user with a valid 20-character username', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { id: 3 } } })
  renderCreateDrawer()

  await submitNewUser('c'.repeat(20))

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({ username: 'c'.repeat(20) }),
      expect.anything()
    )
  )
})
