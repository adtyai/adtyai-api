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
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SignUpForm } from '../components/sign-up-form'

let client: QueryClient

beforeEach(() => {
  window.localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return { data: { success: true, data: { email_verification: false } } }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

async function renderSignUp() {
  const router = createRouter({
    routeTree: createRootRoute({ component: SignUpForm }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

async function submitWithUsername(username: string) {
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Username'), username)
  await user.type(screen.getByLabelText('Password'), 'password-123')
  await user.type(screen.getByLabelText('Confirm password'), 'password-123')
  await user.click(screen.getByRole('button', { name: 'Create account' }))
}

describe('sign-up username rules', () => {
  it('blocks OAuth until site terms are checked and sends that version when starting', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/status') {
        return {
          data: {
            success: true,
            data: {
              site_customer_terms_version: 4,
              discord_oauth: true,
              discord_client_id: 'discord-client',
            },
          },
        }
      }
      throw new Error(`Unexpected request: ${url}`)
    })
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/user/auth/logout') return { data: { success: true } }
      return {
        data: {
          success: true,
          data: {
            flow_token: 'checked-terms',
            redirect_uri: 'https://platform.example/oauth/discord',
          },
        },
      }
    })
    vi.spyOn(window, 'open').mockReturnValue(null)
    await renderSignUp()
    const button = await screen.findByRole('button', {
      name: /Continue with Discord/,
    })
    expect(button).toBeDisabled()
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('checkbox', {
        name: 'I agree to the site customer terms',
      })
    )
    await user.click(button)

    expect(post).toHaveBeenCalledWith(
      '/api/oauth/state',
      expect.objectContaining({ site_terms_version: '4' }),
      expect.anything()
    )
  })

  it('rejects a username containing @ without sending the request', async () => {
    const post = vi.spyOn(api, 'post')
    await renderSignUp()

    await submitWithUsername('victim@example.com')

    expect(
      await screen.findByText('Username cannot contain @')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Username')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(post).not.toHaveBeenCalled()
  })

  it('rejects a username longer than 20 characters without sending the request', async () => {
    const post = vi.spyOn(api, 'post')
    await renderSignUp()

    await submitWithUsername('a'.repeat(21))

    expect(
      await screen.findByText('Username must be 20 characters or fewer')
    ).toBeInTheDocument()
    expect(post).not.toHaveBeenCalled()
  })

  it('sends a 20-character username to the register endpoint', async () => {
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: false, message: 'stop here' } })
    await renderSignUp()

    await submitWithUsername('b'.repeat(20))

    await vi.waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post.mock.calls[0][0]).toBe('/api/user/register')
    expect(post.mock.calls[0][1]).toMatchObject({ username: 'b'.repeat(20) })
  })

  it('sends the current site customer terms version after the box is checked', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/status') {
        return {
          data: {
            success: true,
            data: { email_verification: false, site_customer_terms_version: 4 },
          },
        }
      }
      throw new Error(`Unexpected request: ${url}`)
    })
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: false, message: 'stop here' } })
    await renderSignUp()

    expect(
      await screen.findByRole('checkbox', {
        name: 'I agree to the site customer terms',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create account' })
    ).toBeDisabled()

    const user = userEvent.setup()
    await user.type(await screen.findByLabelText('Username'), 'sitecustomer')
    await user.type(screen.getByLabelText('Password'), 'password-123')
    await user.type(screen.getByLabelText('Confirm password'), 'password-123')
    await user.click(
      screen.getByRole('checkbox', {
        name: 'I agree to the site customer terms',
      })
    )
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await vi.waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post.mock.calls[0][2]).toMatchObject({
      params: { site_terms_version: '4' },
    })
  })
})
