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
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useApiBaseUrl } from '../api'

const http = vi.hoisted(() => ({ get: vi.fn(), getStatus: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  api: { get: http.get },
  getStatus: http.getStatus,
}))

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

function siteResponse(apiBaseUrl: string) {
  return {
    data: {
      success: true,
      data: { api_base_url: apiBaseUrl, refund_policy_enabled: false },
    },
  }
}

afterEach(() => {
  window.localStorage.clear()
})

describe('useApiBaseUrl', () => {
  it('uses the site API address without a trailing slash', async () => {
    http.get.mockResolvedValue(siteResponse('https://api.example.com/'))
    http.getStatus.mockResolvedValue({
      server_address: 'https://srv.example.com',
    })
    const { result } = renderHook(() => useApiBaseUrl(), { wrapper })
    await waitFor(() => expect(result.current).toBe('https://api.example.com'))
    expect(http.get).toHaveBeenCalledWith('/api/adty/site')
  })

  it('falls back to the status server address when the site address is empty', async () => {
    http.get.mockResolvedValue(siteResponse('  '))
    http.getStatus.mockResolvedValue({
      server_address: 'https://srv.example.com',
    })
    const { result } = renderHook(() => useApiBaseUrl(), { wrapper })
    await waitFor(() => expect(result.current).toBe('https://srv.example.com'))
  })

  it('falls back to the current origin when nothing is configured', async () => {
    http.get.mockRejectedValue(new Error('offline'))
    http.getStatus.mockResolvedValue({})
    const { result } = renderHook(() => useApiBaseUrl(), { wrapper })
    await waitFor(() => expect(http.getStatus).toHaveBeenCalled())
    expect(result.current).toBe(window.location.origin)
  })
})
