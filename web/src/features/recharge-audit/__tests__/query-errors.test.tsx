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

import { RechargeAudit } from '../index'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it.each(['orders', 'summary', 'orphans'])(
  'retries recharge audit %s failures',
  async (section) => {
    let failed = true
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      let target = path.endsWith('/summary')
      if (section === 'orders') target = path.endsWith('/recharge-audit')
      else if (section === 'orphans') target = path.includes('orphans')
      if (target && failed) throw new Error('offline')
      return {
        data: {
          success: true,
          data: {
            items: [],
            total: 0,
            orders: 0,
            credited_usd: '0',
            bonus_usd: '0',
          },
        },
      }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <RechargeAudit />
      </QueryClientProvider>
    )
    const retry = await screen.findByRole('button', { name: 'Retry' })
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    client.clear()
  }
)
