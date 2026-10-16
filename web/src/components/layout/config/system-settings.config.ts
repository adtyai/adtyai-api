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
import {
  Box,
  Route,
  CreditCard,
  Layout,
  Settings,
  Shield,
  ShieldAlert,
  Wrench,
} from 'lucide-react'

import { getAuthSectionNavItems } from '@/features/system-settings/auth/section-registry.tsx'
import { getBillingSectionNavItems } from '@/features/system-settings/billing/section-registry.tsx'
import { getContentSectionNavItems } from '@/features/system-settings/content/section-registry.tsx'
import { getModelsSectionNavItems } from '@/features/system-settings/models/section-registry.tsx'
import { getOperationsSectionNavItems } from '@/features/system-settings/operations/section-registry.tsx'
import { getPolicySectionNavItems } from '@/features/system-settings/request-policies/section-registry'
import { getSecuritySectionNavItems } from '@/features/system-settings/security/section-registry.tsx'
import { getSiteSectionNavItems } from '@/features/system-settings/site/section-registry.tsx'

import type { SidebarBadges } from '@/features/system-settings/sidebar-badges/types'

import type { NavGroup, NavItem, SidebarView } from '../types'

export function pendingBadge(count: number | undefined): string | undefined {
  if (count === undefined || !Number.isFinite(count) || count <= 0) return undefined
  return count > 99 ? '99+' : String(Math.floor(count))
}

/** Root「系统设置」角标 = risk + withdraw + topup 合计；无待办则不显。 */
export function sumSidebarBadges(badges?: SidebarBadges): string | undefined {
  if (!badges) return undefined
  return pendingBadge(
    (badges.risk_pending || 0) +
      (badges.withdraw_pending || 0) +
      (badges.topup_pending || 0)
  )
}

/** Parse child badge text ("12", "99+") into a countable integer. */
function parseBadgeCount(badge: string | undefined): number {
  if (!badge) return 0
  const n = Number.parseInt(badge, 10)
  return Number.isFinite(n) && n > 0 ? n : 0
}

/**
 * Parent (collapsible) badge = sum of child pending badges.
 * Shown when collapsed so pending work is visible without expanding.
 */
function withParentBadges(items: NavItem[]): NavItem[] {
  return items.map((item) => {
    if (!('items' in item) || !item.items?.length) return item
    const sum = item.items.reduce((acc, sub) => acc + parseBadgeCount(sub.badge), 0)
    const badge = pendingBadge(sum)
    if (!badge) return item
    return { ...item, badge }
  })
}

/**
 * Sidebar nav groups for the System Settings nested view.
 *
 * Official section trees stay intact; ADTY 二开 entries are nested under
 * the matching official parent (Site / Billing / Security / Operations)
 * so the drill-in sidebar is not a long flat list of custom items.
 */
export function getSystemSettingsNavGroups(
  t: TFunction,
  badges?: SidebarBadges
): NavGroup[] {
  return [
    {
      id: 'system-administration',
      title: t('System Administration'),
      items: withParentBadges([
        {
          title: t('Site & Branding'),
          icon: Settings,
          items: [
            ...getSiteSectionNavItems(t),
            // 菜单显隐已并入「侧边栏模块」页，不再单独占侧栏项
          ],
        },
        {
          title: t('Authentication'),
          icon: Shield,
          items: getAuthSectionNavItems(t),
        },
        {
          title: t('Billing & Payment'),
          icon: CreditCard,
          items: [
            ...getBillingSectionNavItems(t),
            // 易支付 / BEpusdt 已并入「支付网关」页，不再单独占侧栏项
            // 分组显示名已并入「分组定价」页，不再单独占侧栏项
            // 充值/退款/赠送/返佣业务入口并入支付树，去掉独立顶栏项
            {
              title: t('Topup orders'),
              url: '/system-settings/orders',
              badge: pendingBadge(badges?.topup_pending),
            },
            {
              title: t('Order refunds'),
              url: '/system-settings/order-refunds',
            },
            {
              title: t('Bonus ledger'),
              url: '/system-settings/bonus-ledger',
            },
            {
              title: t('Affiliate commissions'),
              url: '/system-settings/aff-commissions',
            },
            {
              title: t('Affiliate withdrawals'),
              url: '/system-settings/aff-withdrawals',
              badge: pendingBadge(badges?.withdraw_pending),
            },
          ],
        },
        {
          title: t('Models'),
          icon: Box,
          items: getModelsSectionNavItems(t),
        },
        {
          title: t('Request policies'),
          icon: Route,
          items: getPolicySectionNavItems(t),
        },
        {
          title: t('Security & Limits'),
          icon: ShieldAlert,
          items: [
            ...getSecuritySectionNavItems(t),
            {
              title: t('Risk reviews'),
              url: '/system-settings/risk-reviews',
              badge: pendingBadge(badges?.risk_pending),
            },
          ],
        },
        {
          title: t('Console Content'),
          icon: Layout,
          items: getContentSectionNavItems(t),
        },
        {
          title: t('Operations'),
          icon: Wrench,
          items: [
            ...getOperationsSectionNavItems(t),
            // 业务参数 / Telegram / Chatwoot → 运维与通知类入口
            {
              title: t('Business parameters'),
              url: '/system-settings/business-params',
            },
            {
              title: t('Telegram alerts'),
              url: '/system-settings/alerts',
            },
            {
              title: t('Chatwoot customer support'),
              url: '/system-settings/chatwoot',
            },
          ],
        },
      ]),
    },
  ]
}

/**
 * Nested sidebar view for `/system-settings/*`.
 *
 * Activates the Vercel / Cloudflare-style drill-in sidebar:
 * the root navigation is replaced by the system administration
 * groups, with a "Back to Dashboard" affordance in the header.
 */
export const SYSTEM_SETTINGS_VIEW: SidebarView = {
  id: 'system-settings',
  pathPattern: /^\/system-settings(\/|$)/,
  parent: {
    to: '/dashboard/overview',
    label: 'Back to Dashboard',
  },
  getNavGroups: getSystemSettingsNavGroups,
}
