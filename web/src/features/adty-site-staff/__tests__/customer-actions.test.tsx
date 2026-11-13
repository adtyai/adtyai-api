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
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { StaffWorkspace } from '../index'

const originalAdapter = api.defaults.adapter
let client: QueryClient
let permissions: string[]
let version: number
let customerStatus: number
let failAction: boolean
let writes: { path: string; body: Record<string, unknown>; proof: unknown }[]

beforeEach(() => {
  permissions = ['customers.read', 'customers.disable']
  version = 1
  customerStatus = 1
  failAction = false
  writes = []
  useAuthStore.getState().auth.setBundle({
    access_token: 'staff-customer-session',
    token_type: 'Bearer',
    access_expires_at: Math.floor(Date.now() / 1000) + 600,
    user: { id: 11, username: 'staff', role: 1, staff_site_id: 7 },
    session: {
      sid: 'staff-customer-session-id',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'staff-test',
      created_at: 1,
      last_active_at: 1,
      expires_at: Math.floor(Date.now() / 1000) + 3600,
    },
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  api.defaults.adapter = (async (config) => {
    const path = config.url ?? ''
    let data: unknown
    let success = true
    if (config.method !== 'get') {
      const body = JSON.parse(config.data)
      writes.push({ path, body, proof: config.headers.get('X-Security-Proof') })
      if (path === '/api/verify') {
        data = {
          proof_token: 'customer-action-proof',
          expires_at: Math.floor(Date.now() / 1000) + 120,
          method: 'password',
          scope: 'site.staff.customer.manage',
        }
      } else {
        success = !failAction
        if (success && path.endsWith('/disable')) customerStatus = 2
        data = null
      }
    } else if (path === '/api/verify/methods') {
      data = {
        scope: 'site.staff.customer.manage',
        methods: [{ method: 'password', available: true }],
        oauth_providers: [],
        password_encryption_enabled: false,
      }
    } else if (path.endsWith('/staff/self')) {
      data = {
        staff: {
          id: 1,
          user_id: 11,
          site_id: 7,
          username: 'staff',
          display_name: 'Support',
          role: 'support',
          status: 'enabled',
          version,
          permissions,
        },
        site_name: 'shop.example',
      }
    } else if (path.endsWith('/staff/customers')) {
      data = {
        total: 1,
        items: [
          {
            id: 20,
            username: 'local-customer',
            email: 'c***@example.com',
            status: customerStatus,
            group: 'default',
            created_at: 1,
            reviewed: false,
          },
        ],
      }
    } else {
      throw new Error(`Unexpected request: ${path}`)
    }
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success, data, message: success ? '' : 'Please try again later' },
    }
  }) satisfies AxiosAdapter
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
})

function renderWorkspace() {
  render(
    <QueryClientProvider client={client}>
      <StaffWorkspace />
    </QueryClientProvider>
  )
}

it('hides both customer write actions without explicit permissions', async () => {
  permissions = ['customers.read']
  renderWorkspace()
  await screen.findByText('local-customer')
  expect(
    screen.queryByRole('button', { name: 'Send password reset instructions' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Disable customer' })
  ).not.toBeInTheDocument()
})

it('does not offer a staff control to send customer password reset instructions', async () => {
  permissions = ['customers.read', 'customers.password', 'customers.disable']
  renderWorkspace()
  await screen.findByText('local-customer')
  expect(
    screen.queryByRole('button', { name: 'Send password reset instructions' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Disable customer' })
  ).toBeInTheDocument()
})

it('cancels verification without changing a customer and closes confirmation after a grant is revoked', async () => {
  const user = userEvent.setup()
  renderWorkspace()
  await user.click(
    await screen.findByRole('button', { name: 'Disable customer' })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  await user.click(await screen.findByRole('button', { name: 'Cancel' }))
  expect(writes).toHaveLength(0)
  expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  permissions = ['customers.read']
  version = 2
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['adty-staff', 11, 'self'] })
  })
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(
    screen.queryByRole('button', { name: 'Disable customer' })
  ).not.toBeInTheDocument()
  expect(writes).toHaveLength(0)
})

it('keeps failed customer actions retryable and hides disable after the refreshed customer is disabled', async () => {
  const user = userEvent.setup()
  failAction = true
  renderWorkspace()
  await user.click(
    await screen.findByRole('button', { name: 'Disable customer' })
  )
  for (const failure of [true, false]) {
    failAction = failure
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Continue',
      })
    )
    await user.type(
      await screen.findByLabelText('Password', { selector: 'input' }),
      'employee-password-42'
    )
    await user.click(screen.getByRole('button', { name: 'Verify' }))
    if (failure) {
      await waitFor(() =>
        expect(
          within(screen.getByRole('alertdialog')).getByRole('button', {
            name: 'Continue',
          })
        ).toBeEnabled()
      )
    } else {
      await waitFor(() =>
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      )
    }
  }
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'Disable customer' })
    ).not.toBeInTheDocument()
  )
  const actions = writes.filter((write) => write.path.endsWith('/disable'))
  expect(actions).toHaveLength(2)
  expect(
    actions.every(
      (write) =>
        Object.keys(write.body).length === 0 &&
        write.proof === 'customer-action-proof'
    )
  ).toBe(true)
})
