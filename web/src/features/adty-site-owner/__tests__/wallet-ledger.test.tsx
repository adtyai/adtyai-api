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
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { WalletLedgerPanel } from '../components/wallet-ledger-panel'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('shows earnings and margin wallets without the operating wallet or operator ids', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/adty/site/wallets') {
      return {
        data: {
          success: true,
          data: [
            { id: 1, kind: 'operating', balance: '999' },
            { id: 2, kind: 'earnings', balance: '20', frozen_usd: '5' },
            { id: 3, kind: 'margin', balance: '100', frozen_usd: '0' },
          ],
        },
      }
    }
    return {
      data: {
        success: true,
        data: {
          total: 1,
          page: 1,
          page_size: 20,
          items: [
            {
              id: 4,
              site_id: 7,
              wallet_id: 2,
              wallet_kind: 'earnings',
              delta: '0',
              hold_delta: '5',
              balance_before: '20',
              balance_after: '20',
              type: 'voucher_freeze',
              ref_type: 'voucher',
              ref_id: 'abc123',
              reason: 'voucher issuance freeze',
              operator_id: 70,
              allow_overdraft: false,
              created_at: 1,
            },
          ],
        },
      },
    }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <WalletLedgerPanel />
    </QueryClientProvider>
  )
  const balances = await screen.findByRole('region', { name: 'Wallets' })
  expect(balances).toHaveTextContent('Earnings wallet')
  expect(balances).toHaveTextContent('20 USD')
  expect(balances).toHaveTextContent('Margin wallet')
  expect(balances).not.toHaveTextContent('Operating wallet')
  expect(balances).not.toHaveTextContent('999')
  const table = await screen.findByRole('table', { name: 'Wallet ledger' })
  expect(table).toHaveTextContent('Recharge code issued')
  expect(table).toHaveTextContent('+5 USD')
  expect(within(table).queryByText('#70')).not.toBeInTheDocument()
  const filter = screen.getByRole('combobox', { name: 'Wallet' })
  expect(
    within(filter)
      .getAllByRole('option')
      .map((option) => option.getAttribute('value'))
  ).toEqual(['', 'earnings', 'margin'])
  expect(get).toHaveBeenCalledWith('/api/adty/site/wallet-ledgers', {
    params: { kind: undefined, p: 1, page_size: 20 },
  })
  client.clear()
})
