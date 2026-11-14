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
import { createInstance } from 'i18next'
import type { i18n as I18n } from 'i18next'
import { useMemo, type ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AdtyPaymentDialogs } from '@/features/wallet/adty/components/adty-payment-dialogs'
import type { ActivePayment } from '@/features/wallet/adty/lib/active-payment'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

import {
  installTawkSettingsLocaleHook,
  officialTawkLocale,
  rewriteTawkWidgetSettings,
} from '../tawk-locale'
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

function createTestI18n(language: string) {
  const instance = createInstance()
  void instance.use(initReactI18next).init({
    lng: language,
    fallbackLng: 'en',
    supportedLngs: ['en', 'zhCN', 'fr', 'zhTW', 'ja'],
    interpolation: { escapeValue: false },
    resources: {
      en: { translation: {} },
      zhCN: { translation: {} },
      fr: { translation: {} },
      zhTW: { translation: {} },
      ja: { translation: {} },
    },
  })
  return instance
}

function wrapperFor(i18n: I18n) {
  return function Wrapper(props: { children: ReactNode }) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    return (
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
      </I18nextProvider>
    )
  }
}

function wrapper(props: { children: ReactNode }) {
  const i18n = useMemo(() => createTestI18n('en'), [])
  return wrapperFor(i18n)(props)
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
  delete window.$_Tawk
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

  it('applies official Simplified Chinese as the Tawk zh_cn locale at load', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    render(<TawkWidget />, { wrapper: wrapperFor(createTestI18n('zhCN')) })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    const script = tawkScripts()[0] as HTMLScriptElement
    expect(script.src).toBe('https://embed.tawk.to/64f0abc/1h9xyz')
    expect(script.dataset.tawkLocale).toBe('zh_cn')
  })

  it('applies official English as the Tawk en locale at load', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    render(<TawkWidget />, { wrapper: wrapperFor(createTestI18n('en')) })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).toBe('en')
  })

  it('updates the widget language when the official UI language changes', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    const i18n = createTestI18n('zhCN')
    render(<TawkWidget />, { wrapper: wrapperFor(i18n) })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).toBe(
      'zh_cn'
    )

    await act(async () => {
      await i18n.changeLanguage('en')
    })
    await waitFor(() =>
      expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).toBe(
        'en'
      )
    )
    expect(tawkScripts()).toHaveLength(1)
    expect((tawkScripts()[0] as HTMLScriptElement).src).toBe(
      'https://embed.tawk.to/64f0abc/1h9xyz'
    )
  })

  it('does not treat unofficial leftover codes as a third official Tawk language', async () => {
    http.get.mockResolvedValue(site('64f0abc', '1h9xyz'))
    const i18n = createTestI18n('fr')
    render(<TawkWidget />, { wrapper: wrapperFor(i18n) })
    await waitFor(() => expect(tawkScripts()).toHaveLength(1))
    expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).toBe('en')

    await act(async () => {
      await i18n.changeLanguage('ja')
    })
    await waitFor(() =>
      expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).toBe(
        'en'
      )
    )

    await act(async () => {
      await i18n.changeLanguage('zhTW')
    })
    await waitFor(() =>
      expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).toBe(
        'zh_cn'
      )
    )
    expect((tawkScripts()[0] as HTMLScriptElement).dataset.tawkLocale).not.toBe(
      'zh_tw'
    )
  })
})

describe('official Tawk locale mapping and settings hook', () => {
  it('maps official UI languages onto Tawk pack tags and coerces leftovers', () => {
    expect(officialTawkLocale('zhCN')).toBe('zh_cn')
    expect(officialTawkLocale('zh-CN')).toBe('zh_cn')
    expect(officialTawkLocale('zh')).toBe('zh_cn')
    expect(officialTawkLocale('zhTW')).toBe('zh_cn')
    expect(officialTawkLocale('zh-HK')).toBe('zh_cn')
    expect(officialTawkLocale('en')).toBe('en')
    expect(officialTawkLocale('en-US')).toBe('en')
    expect(officialTawkLocale('fr')).toBe('en')
    expect(officialTawkLocale('ja')).toBe('en')
    expect(officialTawkLocale(undefined)).toBe('en')
  })

  it('rewrites widget-settings language to the official Tawk tag', () => {
    const rewritten = rewriteTawkWidgetSettings(
      { data: { widget: { language: 'ja', name: 'keep' }, settingsVersion: 4 } },
      'zh_cn'
    ) as { data: { widget: { language: string; name: string } } }
    expect(rewritten.data.widget.language).toBe('zh_cn')
    expect(rewritten.data.widget.name).toBe('keep')
  })

  it('hooks fetch so Tawk widget-settings load the official locale', async () => {
    const previous = window.fetch
    window.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ data: { widget: { language: 'ja' } } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    )
    const restore = installTawkSettingsLocaleHook(() => 'zh_cn')
    try {
      const response = await window.fetch(
        'https://va.tawk.to/v1/widget-settings?propertyId=64f0abc&widgetId=1h9xyz'
      )
      expect(await response.json()).toEqual({
        data: { widget: { language: 'zh_cn' } },
      })
      const other = await window.fetch('https://example.com/other')
      expect(await other.json()).toEqual({ data: { widget: { language: 'ja' } } })
    } finally {
      restore()
      window.fetch = previous
    }
  })
})
