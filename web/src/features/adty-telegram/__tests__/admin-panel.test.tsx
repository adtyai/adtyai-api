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
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { TelegramAdminPanel } from '../admin-panel'

const verification = vi.hoisted(() => ({ requestVerification: vi.fn() }))
vi.mock(
  '@/features/auth/secure-verification/hooks/use-secure-verification',
  () => ({
    useSecureVerification: () => ({
      requestVerification: verification.requestVerification,
      dialogProps: {},
    }),
  })
)
vi.mock(
  '@/features/auth/secure-verification/components/secure-verification-dialog',
  () => ({ SecureVerificationDialog: () => null })
)

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('sends a test from a configured bot with a verification bound to that bot', async () => {
  verification.requestVerification.mockResolvedValue({ proof_token: 'proof' })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        bots: [
          {
            purpose: 'admin',
            token_mask: '****uvwx',
            identity_verified: true,
            saved_at: 1,
            tested_at: 0,
            pending: 0,
          },
          {
            purpose: 'user',
            token_mask: '',
            identity_verified: false,
            saved_at: 0,
            tested_at: 0,
            pending: 0,
          },
        ],
      },
    },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { tested: true } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <TelegramAdminPanel />
    </QueryClientProvider>
  )
  const buttons = await screen.findAllByRole('button', { name: 'Send test' })
  expect(buttons[0]).toBeEnabled()
  expect(buttons[1]).toBeDisabled()
  await userEvent.setup().click(buttons[0])
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/admin/telegram/bots/admin/test',
      undefined,
      { headers: { 'X-Security-Proof': 'proof' } }
    )
  )
  expect(verification.requestVerification).toHaveBeenCalledWith(
    expect.objectContaining({
      context: { action: 'telegram.test', target: 'admin' },
    })
  )
})
