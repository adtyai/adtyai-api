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
import { act, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AdtyPaymentDialogs } from '@/features/wallet/adty/components/adty-payment-dialogs'
import type { ActivePayment } from '@/features/wallet/adty/lib/active-payment'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

import { TawkWidget } from '../tawk-widget'

const http = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  api: { get: http.get },
}))
// The payment dialog only needs an order request that never settles.
vi.mock('@/features/wallet/adty/api-pay', () => ({
  payWithBEpusdt: () => new Promise(() => {}),
  payWithEpay: () => new Promise(() => {}),
}))

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

function site(propertyId: string, widgetId: string) {
  return {
    data: {
      success: true,
      data: {
        api_base_url: '',
        support_email: '',
        telegram_url: '',
        support_hours: '',
        tawk_property_id: propertyId,
        tawk_widget_id: widgetId,
        refund_policy_enabled: false,
      },
    },
  }
}

function tawkScripts() {
  return document.querySelectorAll('script[src^="https://embed.tawk.to/"]')
}

beforeEach(() => {
  useAdtyUiStore.setState({ paymentDialogOpen: false })
})

afterEach(() => {
  vi.clearAllMocks()
  tawkScripts().forEach((node) => node.remove())
  delete window.Tawk_API
  delete window.Tawk_LoadStart
})

describe('TawkWidget', () => {
  it('does not insert the script when the ids are not configured', async () => {
    http.get.mockResolvedValue(site('abc123', ''))
    render(<TawkWidget />, { wrapper })
    await waitFor(() => expect(http.get).toHaveBeenCalledWith('/api/adty/site'))
    await act(async () => {})
    expect(tawkScripts()).toHaveLength(0)
  })

  it('does not insert the script for ids that are not plain tokens', async () => {
    http.get.mockResolvedValue(site('abc"><x', 'default'))
    render(<TawkWidget />, { wrapper })
    await waitFor(() => expect(http.get).toHaveBeenCalled())
    await act(async () => {})
    expect(tawkScripts()).toHaveLength(0)
  })

  it('inserts the embed script once when both ids are configured', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    const first = render(<TawkWidget />, { wrapper })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    const script = tawkScripts()[0] as HTMLScriptElement
    expect(script.src).toBe('https://embed.tawk.to/64f0abc/1h9xyz')
    expect(script.async).toBe(true)

    first.unmount()
    render(<TawkWidget />, { wrapper })
    await act(async () => {})
    expect(tawkScripts()).toHaveLength(1)
  })

  it('hides the widget while a payment dialog is open and shows it after', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    render(<TawkWidget />, { wrapper })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    const hideWidget = vi.fn()
    const showWidget = vi.fn()
    window.Tawk_API = { ...window.Tawk_API, hideWidget, showWidget }

    act(() => useAdtyUiStore.getState().setPaymentDialogOpen(true))
    expect(hideWidget).toHaveBeenCalledTimes(1)
    expect(showWidget).not.toHaveBeenCalled()

    act(() => useAdtyUiStore.getState().setPaymentDialogOpen(false))
    expect(showWidget).toHaveBeenCalledTimes(1)
  })

  it('hides the widget on load when a payment dialog is already open', async () => {
    useAdtyUiStore.setState({ paymentDialogOpen: true })
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    render(<TawkWidget />, { wrapper })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    const hideWidget = vi.fn()
    window.Tawk_API = { ...window.Tawk_API, hideWidget, showWidget: vi.fn() }

    act(() => window.Tawk_API?.onLoad?.())
    expect(hideWidget).toHaveBeenCalledTimes(1)
  })

  it('hides the widget when the USDT payment dialog opens and shows it when it closes', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    const payment: ActivePayment = {
      kind: 'usdt',
      method: 'usdt.trc20',
      amount: 10,
    }
    const view = render(
      <>
        <TawkWidget />
        <AdtyPaymentDialogs
          payment={null}
          onClose={() => {}}
          onSettled={() => {}}
        />
      </>,
      { wrapper }
    )
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    const hideWidget = vi.fn()
    const showWidget = vi.fn()
    window.Tawk_API = { ...window.Tawk_API, hideWidget, showWidget }

    view.rerender(
      <>
        <TawkWidget />
        <AdtyPaymentDialogs
          payment={payment}
          onClose={() => {}}
          onSettled={() => {}}
        />
      </>
    )
    await waitFor(() => expect(hideWidget).toHaveBeenCalledTimes(1))
    expect(showWidget).not.toHaveBeenCalled()

    view.rerender(
      <>
        <TawkWidget />
        <AdtyPaymentDialogs
          payment={null}
          onClose={() => {}}
          onSettled={() => {}}
        />
      </>
    )
    await waitFor(() => expect(showWidget).toHaveBeenCalledTimes(1))
  })
})
