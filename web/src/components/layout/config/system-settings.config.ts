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
  Activity,
  LayoutDashboard,
  Bell,
  ClipboardList,
  Box,
  Route,
  Coins,
  CreditCard,
  Layout,
  Settings,
  Shield,
  ShieldAlert,
  SlidersHorizontal,
  Wallet,
  HandCoins,
  Undo2,
  Receipt,
  Gift,
  MessageCircle,
  Tags,
  UserPlus,
  UsersRound,
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
import { ROLE } from '@/lib/roles'

import type { NavGroup, SidebarView } from '../types'

/**
 * Sidebar nav groups for the System Settings nested view.
 *
 * Kept as a single group because the workspace title in the sidebar
 * header already provides top-level context — the inner group label
 * scopes the items as "administration" actions.
 */
function getSystemSettingsNavGroups(t: TFunction): NavGroup[] {
  return [
    {
      id: 'system-administration',
      title: t('System Administration'),
      items: [
        {
          title: t('Operations overview'),
          url: '/system-settings/overview',
          icon: LayoutDashboard,
        },
        {
          title: t('Site & Branding'),
          icon: Settings,
          items: getSiteSectionNavItems(t),
        },
        {
          title: t('Authentication'),
          icon: Shield,
          items: getAuthSectionNavItems(t),
        },
        {
          title: t('Billing & Payment'),
          icon: CreditCard,
          items: getBillingSectionNavItems(t),
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
          items: getSecuritySectionNavItems(t),
        },
        {
          title: t('Console Content'),
          icon: Layout,
          items: getContentSectionNavItems(t),
        },
        {
          title: t('Business parameters'),
          url: '/system-settings/business-params',
          icon: SlidersHorizontal,
        },
        {
          title: t('Risk reviews'),
          url: '/system-settings/risk-reviews',
          icon: ShieldAlert,
        },
        {
          title: t('Epay instances'),
          url: '/system-settings/epay-instances',
          icon: Wallet,
        },
        {
          title: t('BEpusdt gateway'),
          url: '/system-settings/bepusdt',
          icon: Coins,
        },
        {
          title: t('Telegram alerts'),
          url: '/system-settings/alerts',
          icon: Bell,
        },
        {
          title: t('Chatwoot customer support'),
          url: '/system-settings/chatwoot',
          icon: MessageCircle,
        },
        {
          title: t('Menu visibility'),
          url: '/system-settings/menu-visibility',
          icon: Layout,
          requiredRole: ROLE.SUPER_ADMIN,
        },
        {
          title: t('Group display names'),
          url: '/system-settings/groups',
          icon: Tags,
          requiredRole: ROLE.SUPER_ADMIN,
        },
        {
          title: t('Create administrator'),
          url: '/system-settings/admins',
          icon: UserPlus,
          requiredRole: ROLE.SUPER_ADMIN,
        },
        {
          title: t('Service status'),
          url: '/system-settings/status',
          icon: Activity,
        },
        {
          title: t('Daily reconcile'),
          url: '/system-settings/reconcile',
          icon: ClipboardList,
        },
        {
          title: t('Affiliate commissions'),
          url: '/system-settings/aff-commissions',
          icon: UsersRound,
        },
        {
          title: t('Affiliate withdrawals'),
          url: '/system-settings/aff-withdrawals',
          icon: HandCoins,
        },
        {
          title: t('Topup orders'),
          url: '/system-settings/orders',
          icon: Receipt,
        },
        {
          title: t('Bonus ledger'),
          url: '/system-settings/bonus-ledger',
          icon: Gift,
        },
        {
          title: t('Order refunds'),
          url: '/system-settings/order-refunds',
          icon: Undo2,
        },
        // Hidden from sidebar (routes/pages kept): enterprise-pricing, balance-adjustments
        {
          title: t('Operations'),
          icon: Wrench,
          items: getOperationsSectionNavItems(t),
        },
      ],
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
