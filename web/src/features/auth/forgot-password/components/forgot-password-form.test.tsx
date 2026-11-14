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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ForgotPasswordForm } from './forgot-password-form'

afterEach(() => {
  vi.restoreAllMocks()
})

function renderForm() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ForgotPasswordForm />
    </QueryClientProvider>
  )
}

it('asks for a unique username when the email matches more than one account', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: false,
      code: 'EMAIL_AMBIGUOUS',
      message:
        'This email matches more than one account. Enter your unique username, then continue.',
      message_key:
        'This email matches more than one account. Enter your unique username, then continue.',
    },
  })
  const user = userEvent.setup()
  renderForm()
  expect(
    screen.getByText(
      'If this email has more than one account, enter your unique username.'
    )
  ).toBeInTheDocument()
  await user.type(screen.getByLabelText('Email'), 'shared@example.com')
  await user.click(screen.getByRole('button', { name: 'Send reset email' }))
  expect(
    await screen.findByText(
      'This email matches more than one account. Enter your unique username, then continue.'
    )
  ).toBeInTheDocument()
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/reset_password',
      expect.objectContaining({
        params: expect.objectContaining({ email: 'shared@example.com' }),
      })
    )
  )
  expect(get.mock.calls[0][1]?.params?.username).toBeUndefined()

  get.mockResolvedValue({
    data: { success: true, message: '' },
  })
  await user.type(screen.getByLabelText('Username'), 'inherit-closed-user')
  await user.click(screen.getByRole('button', { name: 'Send reset email' }))
  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith(
      '/api/reset_password',
      expect.objectContaining({
        params: expect.objectContaining({
          email: 'shared@example.com',
          username: 'inherit-closed-user',
        }),
      })
    )
  )
})
