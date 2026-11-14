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
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { TranslationAdminPanel } from '../admin-panel'

vi.mock(
  '@/features/auth/secure-verification/hooks/use-secure-verification',
  () => ({
    useSecureVerification: () => ({
      requestVerification: vi.fn(),
      dialogProps: {},
    }),
  })
)
vi.mock(
  '@/features/auth/secure-verification/components/secure-verification-dialog',
  () => ({
    SecureVerificationDialog: () => null,
  })
)

const okConfig = {
  data: {
    success: true,
    data: { saved_at: 1, tested_at: 2, translated_at: 3 },
  },
}

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('TranslationAdminPanel', () => {
  it('shows a loading state while the config request is pending', async () => {
    vi.spyOn(api, 'get').mockImplementation(() => new Promise(() => {}))
    render(<TranslationAdminPanel />, { wrapper })
    expect(await screen.findByText('Loading...')).toBeInTheDocument()
    expect(screen.getByText('System translation')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  })

  it('shows a load failure and retries until the config loads', async () => {
    let failed = true
    vi.spyOn(api, 'get').mockImplementation(async () => {
      if (failed) throw new Error('Failed to load')
      return okConfig
    })
    render(<TranslationAdminPanel />, { wrapper })
    expect(
      await screen.findByText('Oops! Something went wrong')
    ).toBeInTheDocument()
    expect(screen.getByText('Failed to load')).toBeInTheDocument()
    const retry = await screen.findByRole('button', { name: 'Retry' })
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    expect(await screen.findByText(/Saved 1/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
  })
})
