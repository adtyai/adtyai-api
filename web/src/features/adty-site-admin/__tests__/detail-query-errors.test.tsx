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
import { useAuthStore } from '@/stores/auth-store'

import { SiteDetailSheet } from '../components/site-detail-sheet'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

// 同一张面板上有两处「Reason」：站点编辑的 site-reason 和保证金码的
// margin-code-reason。按标签文字找会命中两个，所以用字段 id 定位——
// 这不是产品缺陷，是选择器不够具体。
it.each([
  ['Cost factor', 'site-cost-factor'],
  ['Markup limit', 'site-max-markup'],
  ['Reason', 'site-reason'],
])(
  'confirms before closing site management with an edited %s',
  async (_label, fieldId) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      let data: unknown = {
        site: {
          id: 1,
          status: 'active',
          host: 'alpha.example',
          owner_user_id: 1,
          created_at: 1,
          close_execute_at: 0,
          contact_info: '',
          cost_factor: null,
          max_markup: null,
          brand: '',
          customers: 0,
        },
        domains: [],
        sender_domains: [],
        audits: [],
      }
      if (path.endsWith('/stats')) data = { stats: { total: {} } }
      else if (path.endsWith('/customers')) data = { items: [], total: 0 }
      return { data: { success: true, data } }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const onOpenChange = vi.fn()
    render(
      <QueryClientProvider client={client}>
        <SiteDetailSheet siteId={1} onOpenChange={onOpenChange} />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    const field = () => {
      const element = document.getElementById(fieldId)
      if (!element) throw new Error(`field ${fieldId} is not mounted`)
      return element
    }
    await waitFor(() => expect(field()).toBeInTheDocument())
    await user.type(field(), '1.25')
    await user.keyboard('{Escape}')
    await user.click(await screen.findByRole('button', { name: 'Stay' }))
    expect(field()).toHaveValue('1.25')
    expect(onOpenChange).not.toHaveBeenCalled()
    await user.keyboard('{Escape}')
    await user.click(await screen.findByRole('button', { name: 'Leave' }))
    expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
    client.clear()
  }
)

// 关站对话框现在固定 `transfer_to: 0`（客户由主站继承），页面上已经没有
// 「转移目标」列表，`/sites` 在这次渲染里根本不会被请求，所以原来那条
// transfer-targets 用例测的是一个已移除的控件，永远等不到错误态。
// 其余两条（stats / customers）仍然有效。
it.each(['stats', 'customers'])(
  'retries failed site %s without presenting empty data',
  async (section) => {
    let failed = true
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    const requests = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = String(url)
      const isTarget = path.endsWith(`/${section}`)
      if (isTarget && failed) throw new Error('offline')
      let data: unknown = {
        site: {
          id: 1,
          status: 'active',
          host: 'alpha.example',
          owner_user_id: 1,
          created_at: 1,
          close_execute_at: 0,
          contact_info: '',
          cost_factor: null,
          max_markup: null,
          brand: '',
          customers: 0,
        },
        domains: [],
        sender_domains: [],
        audits: [],
      }
      if (path.endsWith('/stats')) data = { stats: { total: {} } }
      else if (path.endsWith('/customers') || path.endsWith('/sites')) {
        data = { items: [], total: 0 }
      }
      return { data: { success: true, data } }
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <SiteDetailSheet siteId={1} onOpenChange={() => {}} />
      </QueryClientProvider>
    )
    await screen.findByText('alpha.example')
    const retry = await screen.findByRole('button', { name: 'Retry' })
    const before = requests.mock.calls.length
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(requests.mock.calls.length).toBeGreaterThan(before)
    )
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    client.clear()
  }
)
