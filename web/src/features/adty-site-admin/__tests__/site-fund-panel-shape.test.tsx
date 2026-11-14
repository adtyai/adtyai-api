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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SiteFundPanel } from '../components/site-fund-panel'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

/**
 * ISS-R4-0016：`wallets.isSuccess` 为真但响应体不是数组时，面板原来直接 `.map`
 * 会抛 TypeError 让整块资金页白屏。列表形状不可信，就必须兜底。
 */
it.each([
  ['null', null],
  ['an object', { items: [] }],
  ['a string', 'unexpected'],
])(
  'survives a wallets payload that is %s',
  async (_label, walletsPayload) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      if (path.endsWith('/wallets')) {
        return { data: { success: true, data: walletsPayload } }
      }
      if (path.endsWith('/commission-margin')) {
        return { data: { success: true, data: { site_id: 1, allowed: false } } }
      }
      if (path.endsWith('/fund-anomalies')) {
        return { data: { success: true, data: { kinds: [] } } }
      }
      return { data: { success: true, data: { items: [] } } }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <SiteFundPanel siteId={1} />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Wallets')).toBeInTheDocument()
    expect(await screen.findAllByText('No data')).not.toHaveLength(0)
    client.clear()
  }
)

it('still lists the wallets when the payload is an array', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    if (path.endsWith('/wallets')) {
      return {
        data: {
          success: true,
          data: [{ id: 3, kind: 'margin', balance: '12.50' }],
        },
      }
    }
    if (path.endsWith('/commission-margin')) {
      return { data: { success: true, data: { site_id: 1, allowed: false } } }
    }
    if (path.endsWith('/fund-anomalies')) {
      return { data: { success: true, data: { kinds: [] } } }
    }
    return { data: { success: true, data: { items: [] } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteFundPanel siteId={1} />
    </QueryClientProvider>
  )
  expect(await screen.findByText(/^margin 12\.50$/)).toBeInTheDocument()
  client.clear()
})
