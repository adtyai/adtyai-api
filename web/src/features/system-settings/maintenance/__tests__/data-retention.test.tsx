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
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { DataRetentionSection } from '../data-retention-section'

const categories = [
  {
    category: 'user_account_profile',
    title: '用户账号及个人资料',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete', 'deidentify'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Account profile notes',
  },
  {
    category: 'login_security',
    title: '登录/安全记录',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Login notes',
  },
  {
    category: 'api_usage_logs',
    title: 'API 调用日志和用量记录',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    min_retention_days: 90,
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Usage notes',
  },
  {
    category: 'orders_payments',
    title: '订单及支付事件',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Order notes',
  },
  {
    category: 'financial_ledgers',
    title: '资金流水、钱包、保证金、收益、佣金和退款',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Ledger notes',
  },
  {
    category: 'support_tickets',
    title: '客服工单及图片',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Ticket notes',
  },
  {
    category: 'notification_history',
    title: '通知和公告历史',
    auto_cleanable: false,
    configured: false,
    retention_days: null,
    action: 'exclude',
    allowed_actions: ['exclude'],
    default_behavior: 'permanent_retention',
    notes: 'Notification notes',
  },
  {
    category: 'operation_audit',
    title: '操作审计',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Audit notes',
  },
  {
    category: 'task_artifacts',
    title: '任务输入、输出及生成文件关联信息',
    auto_cleanable: true,
    configured: false,
    retention_days: null,
    action: '',
    allowed_actions: ['delete'],
    default_behavior: 'no_automatic_deletion',
    notes: 'Artifact notes',
  },
]

function Fixture() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <DataRetentionSection />
      </SettingsPageProvider>
    </>
  )
}

async function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Fixture />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { categories } },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

test('lists every retention category and does not offer a notification cleanup period', async () => {
  await renderSection()
  expect(
    await screen.findByText(
      'Notification history cannot be given an automatic cleanup period.',
      { exact: false }
    )
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'Permanent retention. Automatic cleanup tasks must not delete notification history. Only the user may manually delete their own deletable messages.'
    )
  ).toBeInTheDocument()
  expect(
    screen.queryByLabelText('Retention period (days)', {
      selector: '#retention-days-notification_history',
    })
  ).not.toBeInTheDocument()
  for (const category of categories) {
    expect(screen.getByText(category.title)).toBeInTheDocument()
  }
  const usageDays = await screen.findByLabelText('Retention period (days)', {
    selector: '#retention-days-api_usage_logs',
  })
  await userEvent.type(usageDays, '180')
  const saveButtons = screen.getAllByRole('button', { name: 'Save' })
  await userEvent.click(saveButtons[2])
  await waitFor(() => {
    expect(api.put).toHaveBeenCalledWith('/api/adty/admin/data-retention', {
      policies: [
        {
          category: 'api_usage_logs',
          retention_days: 180,
          action: 'delete',
        },
      ],
    })
  })
})

test('hides configuration controls from non-Root operators', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  await renderSection()
  expect(
    await screen.findByText('Only Root can view and change data retention periods.')
  ).toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalled()
})
