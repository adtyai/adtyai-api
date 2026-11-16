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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  AuditLogPanel,
  CommissionJobsPanel,
  LedgerPanel,
} from '../components/records-panels'

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

function page<T>(items: T[]) {
  return {
    data: {
      success: true,
      data: { page: 1, page_size: 20, total: items.length, items },
    },
  }
}

const job = {
  source_type: 'topup',
  affiliate_id: 4,
  attempt_count: 2,
  locked_at: 0,
  succeeded_at: 0,
  failed_at: 0,
  created_at: 1,
  updated_at: 1,
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('lists failed commission jobs first and runs one again', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue(
    page([
      {
        ...job,
        id: 1,
        source_trade_no: 'T-FAILED',
        status: 'failed',
        last_error: 'lock timeout',
      },
      {
        ...job,
        id: 2,
        source_trade_no: 'T-DONE',
        status: 'succeeded',
        last_error: '',
      },
    ])
  )
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: null } })
  wrap(<CommissionJobsPanel canRetry />)
  const table = await screen.findByRole('table', { name: 'Commission jobs' })
  expect(get).toHaveBeenCalledWith('/api/user/admin/referral/commission-jobs', {
    params: { page_size: 20, p: 1, status: 'failed' },
  })
  expect(table).toHaveTextContent('lock timeout')
  const done = within(table).getByRole('row', { name: /T-DONE/ })
  expect(within(done).queryByRole('button')).not.toBeInTheDocument()
  const failed = within(table).getByRole('row', { name: /T-FAILED/ })
  await userEvent
    .setup()
    .click(within(failed).getByRole('button', { name: 'Run again' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/admin/referral/commission-jobs/retry',
      { trade_no: 'T-FAILED' }
    )
  )
})

it('hides the retry action without adjust permission', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(
    page([
      {
        ...job,
        id: 1,
        source_trade_no: 'T-FAILED',
        status: 'failed',
        last_error: '',
      },
    ])
  )
  wrap(<CommissionJobsPanel canRetry={false} />)
  const table = await screen.findByRole('table', { name: 'Commission jobs' })
  expect(
    within(table).queryByRole('button', { name: 'Run again' })
  ).not.toBeInTheDocument()
})

it('shows ledger movements with signed deltas and filters by affiliate', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue(
    page([
      {
        id: 7,
        affiliate_id: 4,
        user_id: 40,
        commission_id: 9,
        withdrawal_id: 0,
        type: 'commission_settle',
        ref_type: 'topup',
        ref_id: 'T-1',
        settlement_currency: 'USD',
        delta_pending: -2.5,
        delta_available: 2.5,
        delta_frozen: 0,
        delta_withdrawn: 0,
        remark: '',
        operator: 'system',
        created_at: 1,
      },
    ])
  )
  wrap(<LedgerPanel />)
  const table = await screen.findByRole('table', { name: 'Commission ledger' })
  expect(table).toHaveTextContent('Commission settled')
  expect(table).toHaveTextContent('-2.5')
  expect(table).toHaveTextContent('+2.5')
  expect(table).toHaveTextContent('System')
  await userEvent.setup().type(screen.getByLabelText('Affiliate ID'), '4')
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/user/admin/referral/ledgers', {
      params: { page_size: 20, p: 1, affiliate_id: 4 },
    })
  )
})

it('labels admin actions and shows what changed', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(
    page([
      {
        id: 3,
        action: 'withdraw_cny_rate',
        target_user_id: 0,
        affiliate_id: 0,
        admin_user_id: 1,
        reason: '',
        ip: '127.0.0.1',
        user_agent: 'test',
        old_value: '7.1',
        new_value: '7.2',
        created_at: 1,
      },
    ])
  )
  wrap(<AuditLogPanel />)
  const table = await screen.findByRole('table', { name: 'Admin activity' })
  expect(table).toHaveTextContent('Changed withdrawal CNY rate')
  expect(table).toHaveTextContent('7.1 → 7.2')
  expect(table).toHaveTextContent('#1')
})
