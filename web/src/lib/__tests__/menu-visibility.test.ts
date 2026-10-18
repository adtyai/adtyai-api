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
import { describe, expect, it } from 'vitest'

import type { NavGroup } from '@/components/layout/types'
import {
  filterNavGroupsBySidebarModules,
  type SidebarModulesAdminConfig,
} from '@/hooks/use-sidebar-config'

const defaultAdmin: SidebarModulesAdminConfig = {
  chat: { enabled: true, playground: true, chat: true },
  console: {
    enabled: true,
    detail: true,
    token: true,
    log: true,
    audit: true,
    midjourney: true,
    task: true,
  },
  personal: {
    enabled: true,
    topup: true,
    personal: true,
    security: true,
  },
  admin: {
    enabled: true,
    channel: true,
    smart_routing: true,
    models: true,
    redemption: true,
    user: true,
    setting: true,
    system_info: true,
    task_plugins: true,
    orders: true,
    enterprise_pricing: true,
    alerts: true,
    chatwoot: true,
    risk_reviews: true,
    aff_commissions: true,
    aff_withdrawals: true,
    bonus_ledger: true,
    order_refunds: true,
  },
}

const nested: NavGroup[] = [
  {
    title: 'billing',
    items: [
      { title: 'Orders', url: '/system-settings/orders' },
      { title: 'Chatwoot', url: '/system-settings/chatwoot' },
      { title: 'Telegram', url: '/system-settings/alerts' },
      {
        title: 'Monitoring',
        url: '/system-settings/operations/alerts',
      },
      { title: 'Groups', url: '/system-settings/groups' },
    ],
  },
]

describe('filterNavGroupsBySidebarModules (nested system-settings)', () => {
  it('hides orders when admin.orders=false without touching setting entry', () => {
    const admin: SidebarModulesAdminConfig = {
      ...defaultAdmin,
      admin: { ...defaultAdmin.admin, orders: false, setting: true },
    }
    const out = filterNavGroupsBySidebarModules(nested, admin, null)
    const titles = out[0].items.map((i) => i.title)
    expect(titles).not.toContain('Orders')
    expect(titles).toContain('Chatwoot')
    expect(titles).toContain('Telegram')
  })

  it('hides chatwoot when admin.chatwoot=false', () => {
    const admin: SidebarModulesAdminConfig = {
      ...defaultAdmin,
      admin: { ...defaultAdmin.admin, chatwoot: false },
    }
    const out = filterNavGroupsBySidebarModules(nested, admin, null)
    expect(out[0].items.map((i) => i.title)).not.toContain('Chatwoot')
    expect(out[0].items.map((i) => i.title)).toContain('Orders')
  })

  it('unmapped nested URLs (e.g. operations/alerts) stay visible', () => {
    const admin: SidebarModulesAdminConfig = {
      ...defaultAdmin,
      admin: { ...defaultAdmin.admin, alerts: false },
    }
    const out = filterNavGroupsBySidebarModules(nested, admin, null)
    const titles = out[0].items.map((i) => i.title)
    expect(titles).not.toContain('Telegram')
    expect(titles).toContain('Monitoring')
    expect(titles).toContain('Groups')
  })

  it('admin.setting=false does not hide independent nested modules', () => {
    const admin: SidebarModulesAdminConfig = {
      ...defaultAdmin,
      admin: { ...defaultAdmin.admin, setting: false },
    }
    const out = filterNavGroupsBySidebarModules(nested, admin, null)
    const titles = out[0].items.map((i) => i.title)
    expect(titles).toContain('Orders')
    expect(titles).toContain('Chatwoot')
  })
})

describe('filterNavGroupsBySidebarModules (root)', () => {
  const root: NavGroup[] = [
    {
      id: 'personal',
      title: 'p',
      items: [
        { title: 'Wallet', url: '/wallet' },
        { title: 'Keys', url: '/keys' },
      ],
    },
    {
      id: 'admin',
      title: 'a',
      items: [
        { title: 'Channels', url: '/channels' },
        { title: 'Users', url: '/users' },
      ],
    },
  ]

  it('personal.topup=false hides wallet', () => {
    const admin: SidebarModulesAdminConfig = {
      ...defaultAdmin,
      personal: { ...defaultAdmin.personal, topup: false },
    }
    const out = filterNavGroupsBySidebarModules(root, admin, null)
    expect(out.find((g) => g.id === 'personal')?.items.map((i) => i.title)).toEqual([
      'Keys',
    ])
  })

  it('admin.channel=false hides channels', () => {
    const admin: SidebarModulesAdminConfig = {
      ...defaultAdmin,
      admin: { ...defaultAdmin.admin, channel: false },
    }
    const out = filterNavGroupsBySidebarModules(root, admin, null)
    expect(out.find((g) => g.id === 'admin')?.items.map((i) => i.title)).toEqual([
      'Users',
    ])
  })
})
