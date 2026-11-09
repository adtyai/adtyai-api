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

import { Referral } from '..'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('shows the referral account without offering self-service site creation', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/referral/profile') {
      return {
        data: {
          success: true,
          data: {
            affiliate: {
              invite_code: 'referral-code',
              status: 'approved',
              partner_status: '',
              withdrawal_enabled: true,
            },
            account: {
              pending_amount: 0,
              available_amount: 0,
              frozen_amount: 0,
              withdrawn_amount: 0,
            },
            rate: '10',
            freeze_days: 7,
            min_withdraw_usd: null,
            usdt_withdraw_fee: null,
            cancel_window_minutes: 30,
            cny_rate: '',
            cny_rate_updated_at: 0,
            enabled: true,
            site_owner: false,
            funnel: { clicks: 0, signups: 0, paying: 0, earned: '0' },
          },
        },
      }
    }
    if (url === '/api/adty/site') {
      return { data: { success: true, data: { site_agreement_version: 4 } } }
    }
    if (String(url).startsWith('/api/adty/legal/')) {
      return { data: { success: true, data: 'Current reseller terms' } }
    }
    return { data: { success: true, data: { items: [], total: 0 } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Referral />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Your referral link')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Apply for a reseller site' })
  ).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Subdomain')).not.toBeInTheDocument()
  client.clear()
})
