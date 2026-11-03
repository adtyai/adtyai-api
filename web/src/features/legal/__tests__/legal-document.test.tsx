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
import { act, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RefundPolicy } from '../refund-policy'
import { UserAgreement } from '../user-agreement'

const http = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  api: { get: http.get },
}))
vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children: ReactNode }) => <div>{props.children}</div>,
}))

type Texts = Record<string, Record<string, string>>

/** Serves /api/adty/legal/:doc?lang= from `texts` and /api/adty/site from `site`. */
function serve(texts: Texts, site: { refund_policy_enabled: boolean }) {
  http.get.mockImplementation(
    async (url: string, config?: { params?: { lang?: string } }) => {
      if (url === '/api/adty/site') {
        return { data: { success: true, data: site } }
      }
      const doc = url.replace('/api/adty/legal/', '')
      const lang = config?.params?.lang ?? ''
      return { data: { success: true, data: texts[doc]?.[lang] ?? '' } }
    }
  )
}

function renderPage(page: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(<QueryClientProvider client={client}>{page}</QueryClientProvider>)
}

afterEach(async () => {
  await i18next.changeLanguage('en')
})

describe('LegalDocument', () => {
  it('asks the multilingual endpoint for the current content language', async () => {
    serve(
      { 'user-agreement': { 'zh-CN': '# 用户协议正文' } },
      {
        refund_policy_enabled: false,
      }
    )
    await i18next.changeLanguage('zhCN')
    renderPage(<UserAgreement />)
    expect(await screen.findByText('用户协议正文')).toBeVisible()
    expect(http.get).toHaveBeenCalledWith(
      '/api/adty/legal/user-agreement',
      expect.objectContaining({ params: { lang: 'zh-CN' } })
    )
  })

  it('refetches the text after the language changes', async () => {
    serve(
      {
        'user-agreement': { en: 'English terms', fr: 'Conditions en français' },
      },
      { refund_policy_enabled: false }
    )
    renderPage(<UserAgreement />)
    expect(await screen.findByText('English terms')).toBeVisible()

    await act(() => i18next.changeLanguage('fr'))

    expect(await screen.findByText('Conditions en français')).toBeVisible()
  })
})

describe('RefundPolicy', () => {
  it('shows the refund policy when the site enables it', async () => {
    serve(
      { 'refund-policy': { en: 'Refunds within 7 days' } },
      {
        refund_policy_enabled: true,
      }
    )
    renderPage(<RefundPolicy />)
    expect(await screen.findByText('Refunds within 7 days')).toBeVisible()
  })

  it('shows the empty message without fetching the text when the site disables it', async () => {
    serve(
      { 'refund-policy': { en: 'Refunds within 7 days' } },
      {
        refund_policy_enabled: false,
      }
    )
    renderPage(<RefundPolicy />)
    expect(
      await screen.findByText(
        'The administrator has not configured a refund policy yet.'
      )
    ).toBeVisible()
    expect(screen.queryByText('Refunds within 7 days')).not.toBeInTheDocument()
    expect(http.get).not.toHaveBeenCalledWith(
      '/api/adty/legal/refund-policy',
      expect.anything()
    )
  })
})
