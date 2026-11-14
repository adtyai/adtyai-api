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

import { SiteCommissionPanel, type SiteCommission } from '../site-commission-panel'

const availableRow: SiteCommission = {
  id: 7,
  trade_no: 'SC-7',
  status: 'available',
  amount_usd: '10.00',
  rate: '1',
  clawed_usd: '0',
  converted_usd: '0',
  withdrawn_usd: '0',
  frozen_usd: '0',
  earnings_hold_usd: '0',
  margin_hold_usd: '0',
}

function mockSiteCommissionGet(handler: (url: string) => unknown | Promise<unknown>) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    if (path === '/api/adty/site-commissions') {
      return handler(path)
    }
    return {
      data: { success: true, data: { items: [], methods: [], configured: false } },
    }
  })
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

describe('SiteCommissionPanel', () => {
  it('keeps the commission section visible with a loading state while pending', async () => {
    mockSiteCommissionGet(() => new Promise(() => {}))
    render(<SiteCommissionPanel onConverted={() => {}} />, { wrapper })
    expect(await screen.findByText('Available commission')).toBeInTheDocument()
    expect(screen.getByText('Loading...')).toBeInTheDocument()
    expect(screen.queryByText('No Data')).not.toBeInTheDocument()
  })

  it('shows an empty state when there are no commission rows', async () => {
    mockSiteCommissionGet(() => ({
      data: { success: true, data: { items: [] } },
    }))
    const view = render(<SiteCommissionPanel onConverted={() => {}} />, {
      wrapper,
    })
    expect(await screen.findByText('Available commission')).toBeInTheDocument()
    expect(await screen.findByText('No Data')).toBeInTheDocument()
    expect(view.container.querySelector('section')).not.toBeNull()
    expect(screen.queryByText('SC-7')).not.toBeInTheDocument()
  })

  it('shows a load failure and retries until rows appear', async () => {
    let failed = true
    mockSiteCommissionGet(() => {
      if (failed) throw new Error('offline')
      return { data: { success: true, data: { items: [availableRow] } } }
    })
    render(<SiteCommissionPanel onConverted={() => {}} />, { wrapper })
    expect(
      await screen.findByText('Oops! Something went wrong')
    ).toBeInTheDocument()
    expect(screen.getByText('Available commission')).toBeInTheDocument()
    const retry = await screen.findByRole('button', { name: 'Retry' })
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    expect(await screen.findByText(/SC-7/)).toBeInTheDocument()
  })

  it('disables convert until the inviter enters an amount', async () => {
    mockSiteCommissionGet(() => ({
      data: { success: true, data: { items: [availableRow] } },
    }))
    render(<SiteCommissionPanel onConverted={() => {}} />, { wrapper })
    const convert = await screen.findByRole('button', {
      name: 'Convert commission to balance',
    })
    expect(convert).toBeDisabled()
    await userEvent.setup().type(
      screen.getByLabelText('Amount'),
      '1.00'
    )
    expect(convert).toBeEnabled()
  })
})
