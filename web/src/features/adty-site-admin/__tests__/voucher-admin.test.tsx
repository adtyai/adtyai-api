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

import { VoucherAdminPanel } from '../components/voucher-admin-panel'

const vouchers = [
  {
    id: 11,
    code_tail: 'A1B2',
    purpose: 'main_user',
    status: 'pending',
    amount_usd: '12.5',
    quota: 6250000,
    source_kind: 'platform',
    source_site_id: 0,
    target_site_id: 0,
    issuer_id: 1,
    redeemer_id: 0,
    inherited_by_main: false,
    trade_no: '',
    created_at: 1,
    redeemed_at: 0,
  },
  {
    id: 12,
    code_tail: 'C3D4',
    purpose: 'site_customer',
    status: 'redeemed',
    amount_usd: '4',
    quota: 2000000,
    source_kind: 'earnings',
    source_site_id: 7,
    target_site_id: 0,
    issuer_id: 70,
    redeemer_id: 701,
    inherited_by_main: false,
    trade_no: 'VCH1',
    created_at: 1,
    redeemed_at: 2,
  },
]

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <VoucherAdminPanel />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/adty/admin/vouchers/limit') {
      return {
        data: {
          success: true,
          data: { limit_usd: '100', frozen_usd: '12.5', spent_usd: '4' },
        },
      }
    }
    return { data: { success: true, data: { total: 2, items: vouchers } } }
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('shows the issuance limit and saves a new one', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: {} } })
  show()
  expect(await screen.findByText('100 USD')).toBeVisible()
  expect(screen.getByText('12.5 USD')).toBeVisible()
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('New limit (USD)'), '200')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/admin/vouchers/limit', {
      limit_usd: '200',
    })
  )
})

it('issues a main-site user code and shows it once', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { id: 13, code: 'VCH-FULL-CODE', code_tail: 'CODE' },
    },
  })
  show()
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Amount (USD)'), '5')
  await user.type(screen.getByLabelText('Reason'), 'promo')
  await user.click(screen.getByRole('button', { name: 'Issue code' }))
  expect(await screen.findByText('VCH-FULL-CODE')).toBeVisible()
  expect(api.post).toHaveBeenCalledWith('/api/adty/admin/vouchers', {
    purpose: 'main_user',
    amount_usd: '5',
    reason: 'promo',
  })
})

it('freezes an unredeemed code after confirming and offers nothing on redeemed codes', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: {} } })
  show()
  const user = userEvent.setup()
  const table = await screen.findByRole('table', { name: 'Issued codes' })
  const redeemed = within(table).getByRole('row', { name: /C3D4/ })
  expect(within(redeemed).queryByRole('button')).not.toBeInTheDocument()
  const pending = within(table).getByRole('row', { name: /A1B2/ })
  await user.click(within(pending).getByRole('button', { name: 'Freeze' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Freeze this code?',
  })
  await user.click(within(confirm).getByRole('button', { name: 'Freeze' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/adty/admin/vouchers/11/freeze')
  )
})
