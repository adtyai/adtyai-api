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
import type { TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'


import type { NavItem } from '../../types'
import { SYSTEM_SETTINGS_VIEW } from '../system-settings.config'

const t = ((key: string) => key) as unknown as TFunction

function flattenLinks(items: NavItem[]): Array<{ url?: string; requiredRole?: number; title: string }> {
  const out: Array<{ url?: string; requiredRole?: number; title: string }> = []
  for (const item of items) {
    if ('items' in item && item.items) {
      for (const sub of item.items) {
        out.push({
          url: sub.url as string | undefined,
          requiredRole: sub.requiredRole,
          title: sub.title,
        })
      }
      continue
    }
    if ('url' in item) {
      out.push({
        url: item.url as string | undefined,
        requiredRole: item.requiredRole,
        title: item.title,
      })
    }
  }
  return out
}

describe('system-settings Root-only nav (A39/A08) and no A30 audit', () => {
  it('menu-visibility/groups sidebar gone; group-pricing kept; audit entry is gone', () => {
    const groups = SYSTEM_SETTINGS_VIEW.getNavGroups(t)
    const links = flattenLinks(groups.flatMap((g) => g.items))
    const menu = links.find((i) => i.url === '/system-settings/menu-visibility')
    const group = links.find((i) => i.url === '/system-settings/groups')
    const groupPricing = links.find(
      (i) => i.url === '/system-settings/billing/group-pricing'
    )
    const audit = links.find((i) => i.url === '/system-settings/audit')
    const admins = links.find((i) => i.url === '/system-settings/admins')
    const status = links.find((i) => i.url === '/system-settings/status')
    const reconcile = links.find((i) => i.url === '/system-settings/reconcile')
    const sidebarModules = links.find(
      (i) => i.url === '/system-settings/site/sidebar-modules'
    )

    expect(menu).toBeUndefined()
    expect(group).toBeUndefined()
    expect(groupPricing).toBeDefined()
    expect(sidebarModules).toBeDefined()
    expect(admins).toBeUndefined()
    expect(status).toBeUndefined()
    expect(reconcile).toBeUndefined()
    expect(audit).toBeUndefined()
  })

  it('二开项挂在官方父级下，不再作为顶栏独立项', () => {
    const groups = SYSTEM_SETTINGS_VIEW.getNavGroups(t)
    const top = groups.flatMap((g) => g.items)

    // 顶层只剩官方折叠 + 运营总览，无独立二开 url
    const topUrls = top
      .filter((i) => 'url' in i && i.url)
      .map((i) => i.url as string)
    expect(topUrls).toEqual(['/system-settings/overview'])

    const byParent = Object.fromEntries(
      top
        .filter((i) => 'items' in i && i.items)
        .map((i) => [i.title, (i.items ?? []).map((s) => s.url as string)])
    )

    expect(byParent['Site & Branding']).not.toContain(
      '/system-settings/menu-visibility'
    )
    expect(byParent['Site & Branding']).toContain(
      '/system-settings/site/sidebar-modules'
    )
    expect(byParent['Billing & Payment']).not.toContain(
      '/system-settings/epay-instances'
    )
    expect(byParent['Billing & Payment']).not.toContain(
      '/system-settings/bepusdt'
    )
    expect(byParent['Billing & Payment']).toContain(
      '/system-settings/billing/payment'
    )
    expect(byParent['Billing & Payment']).not.toContain(
      '/system-settings/groups'
    )
    expect(byParent['Billing & Payment']).toContain(
      '/system-settings/billing/group-pricing'
    )
    expect(byParent['Billing & Payment']).toEqual(
      expect.arrayContaining([
        '/system-settings/orders',
        '/system-settings/order-refunds',
        '/system-settings/bonus-ledger',
        '/system-settings/aff-commissions',
        '/system-settings/aff-withdrawals',
      ])
    )
    expect(byParent['Security & Limits']).toContain(
      '/system-settings/risk-reviews'
    )
    expect(byParent['Operations']).toEqual(
      expect.arrayContaining([
        '/system-settings/business-params',
        '/system-settings/alerts',
        '/system-settings/chatwoot',
      ])
    )
  })
})
