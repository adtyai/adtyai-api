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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SiteWithdrawalsPanel } from '../components/site-withdrawals-panel'

const base = {
  site_id: 7,
  owner_user_id: 70,
  subdomain: 'north',
  fee_usd: '1',
  account_name: '',
  tx_id: '',
  proof_note: '',
  reviewer_id: 0,
  reviewed_at: 0,
  paid_at: 0,
  created_at: 1,
}
const rows = [
  {
    ...base,
    id: 1,
    amount_usd: '20',
    net_usd: '19',
    method: 'usdt',
    network: 'TRC20',
    account_no: 'T-address',
    rate: '1',
    payout_amount: '19',
    payout_currency: 'USDT',
    status: 'paying',
  },
  {
    ...base,
    id: 2,
    amount_usd: '10',
    net_usd: '9',
    method: 'alipay',
    network: '',
    account_no: 'owner@example.com',
    rate: '7',
    payout_amount: '63',
    payout_currency: 'CNY',
    status: 'pending',
  },
]

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteWithdrawalsPanel />
    </QueryClientProvider>
  )
}

let post: ReturnType<typeof vi.spyOn>
beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/adty/admin/site-withdrawals') {
      return { data: { success: true, data: { total: 2, items: rows } } }
    }
    return { data: { success: true, data: {} } }
  })
  post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: { updated: true } } })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('lists pending withdrawals by default with the payee and payout', async () => {
  show()
  const table = await screen.findByRole('table', { name: 'Site withdrawals' })
  expect(table).toHaveTextContent('north')
  expect(table).toHaveTextContent('19 USDT')
  expect(table).toHaveTextContent('T-address')
  expect(api.get).toHaveBeenCalledWith('/api/adty/admin/site-withdrawals', {
    params: { status: 'pending', p: 1, page_size: 20 },
  })
})

it('marks a pending request as paying after confirming', async () => {
  show()
  const user = userEvent.setup()
  const row = await screen.findByRole('row', { name: /owner@example.com/ })
  await user.click(within(row).getByRole('button', { name: 'Mark paying' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Mark as paying',
  })
  await user.click(within(confirm).getByRole('button', { name: 'Mark paying' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/admin/site-withdrawals/2/paying'
    )
  )
})

it('requires the transaction hash before recording a USDT payout', async () => {
  show()
  const user = userEvent.setup()
  const row = await screen.findByRole('row', { name: /T-address/ })
  await user.click(within(row).getByRole('button', { name: 'Record payout' }))
  const dialog = await screen.findByRole('dialog', { name: 'Record payout' })
  const save = within(dialog).getByRole('button', { name: 'Record payout' })
  await user.type(
    within(dialog).getByLabelText('Transfer reference or note'),
    'paid'
  )
  expect(save).toBeDisabled()
  await user.type(within(dialog).getByLabelText('Transaction hash'), '0xabc')
  await user.click(save)
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/admin/site-withdrawals/1/paid',
      {
        tx_id: '0xabc',
        proof: 'paid',
      }
    )
  )
})

it('requires a reason before rejecting', async () => {
  show()
  const user = userEvent.setup()
  const row = await screen.findByRole('row', { name: /owner@example.com/ })
  await user.click(within(row).getByRole('button', { name: 'Reject' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Reject withdrawal',
  })
  const reject = within(dialog).getByRole('button', { name: 'Reject' })
  expect(reject).toBeDisabled()
  await user.type(
    within(dialog).getByLabelText('Reason (required)'),
    'wrong account'
  )
  await user.click(reject)
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/admin/site-withdrawals/2/reject',
      { reason: 'wrong account' }
    )
  )
})
