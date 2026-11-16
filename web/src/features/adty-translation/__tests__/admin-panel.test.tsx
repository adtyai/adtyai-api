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
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
  })

  it('saves the loaded configuration instead of overwriting it with fixed values', async () => {
    verification.requestVerification.mockResolvedValue({ proof_token: 'proof' })
    vi.spyOn(api, 'get').mockImplementation(async (url) =>
      url === '/api/adty/admin/translation/usage'
        ? {
            data: {
              success: true,
              data: { day: '2026-11-17', calls: 4, failures: 1, latency_ms: 800 },
            },
          }
        : {
            data: {
              success: true,
              data: {
                mode: 'independent',
                protocol: 'anthropic',
                base_url: 'https://translate.example.com',
                model_name: 'translator-v2',
                backup_model: 'translator-v1',
                token_mask: '****abcd',
                timeout_sec: 20,
                concurrency: 3,
                max_chars: 4000,
                daily_quota: 50,
                retry_limit: 2,
                enabled: true,
                saved_at: 1,
                tested_at: 0,
                translated_at: 0,
              },
            },
          }
    )
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, data: {} } })
    render(<TranslationAdminPanel />, { wrapper })
    expect(await screen.findByText(/Calls 4/)).toBeInTheDocument()
    expect(screen.getByText(/Average latency 200/)).toBeInTheDocument()
    const user = userEvent.setup()
    const model = await screen.findByLabelText('Model')
    expect(model).toHaveValue('translator-v2')
    await user.clear(model)
    await user.type(model, 'translator-v3')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/adty/admin/translation',
        {
          mode: 'independent',
          protocol: 'anthropic',
          base_url: 'https://translate.example.com',
          token: '',
          model_name: 'translator-v3',
          backup_model: 'translator-v1',
          timeout_sec: 20,
          concurrency: 3,
          max_chars: 4000,
          daily_quota: 50,
          retry_limit: 2,
          enabled: true,
        },
        { headers: { 'X-Security-Proof': 'proof' } }
      )
    )
    expect(verification.requestVerification).toHaveBeenCalledWith(
      expect.objectContaining({
        scope: 'adty.notice.configure',
        context: { action: 'translation.save', target: 'independent' },
      })
    )
  })

  it('tests the saved translator with its own verification', async () => {
    verification.requestVerification.mockResolvedValue({ proof_token: 'proof' })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { mode: 'platform', model_name: 'translator', saved_at: 1 },
      },
    })
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true, data: {} } })
    render(<TranslationAdminPanel />, { wrapper })
    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'Test connection' }))
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(
        '/api/adty/admin/translation/test',
        undefined,
        { headers: { 'X-Security-Proof': 'proof' } }
      )
    )
    expect(verification.requestVerification).toHaveBeenCalledWith(
      expect.objectContaining({
        context: { action: 'translation.test', target: 'translation' },
      })
    )
  })
})
