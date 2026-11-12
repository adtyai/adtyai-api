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
import {
  Activity,
  Box,
  ClipboardList,
  FileText,
  Key,
  LifeBuoy,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  PlugZap,
  Radio,
  ServerCog,
  Settings,
  ShieldCheck,
  User,
  Users,
  Wallet,
  ReceiptText,
  Gift,
  HandCoins,
  Store,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { useAdminReferralBadge } from '@/features/admin-referral/hooks/use-admin-referral-badge'
import { ADTY_PERMISSIONS } from '@/lib/adty-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  // ADTY task 8.2: titles and order follow the console mockup. Group ids are
  // unchanged because module switches and the admin-only filter key on them.
  const { t } = useTranslation()
  const referralBadge = useAdminReferralBadge()
  const isSiteOwner = useAuthStore(
    (state) => state.auth.user?.adty_site_owner === true
  )
  const isStaff = useAuthStore(
    (state) => (state.auth.user?.staff_site_id ?? 0) > 0
  )

  return useMemo(() => {
    if (isStaff) {
      return {
        navGroups: [
          {
            id: 'personal',
            title: t('Account'),
            items: [
              { title: t('Staff workspace'), url: '/site-staff', icon: Users },
            ],
          },
        ],
      }
    }
    return {
      navGroups: [
        {
          id: 'general',
          title: t('Console'),
          items: [
            {
              title: t('Overview'),
              url: '/dashboard/overview',
              icon: Activity,
            },
            {
              title: t('Dashboard'),
              url: '/dashboard/models',
              icon: LayoutDashboard,
            },
            {
              title: t('API Keys'),
              url: '/keys',
              icon: Key,
            },
            {
              title: t('Usage Logs'),
              url: '/usage-logs/common',
              icon: FileText,
            },
            {
              title: t('Audit Logs'),
              url: '/usage-logs/audit',
              icon: ClipboardList,
            },
            {
              title: t('Task Logs'),
              url: '/usage-logs/task',
              activeUrls: ['/usage-logs/drawing'],
              configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
              icon: ListTodo,
            },
          ],
        },
        {
          id: 'chat',
          title: t('Chat'),
          items: [
            {
              title: t('Chat'),
              icon: MessageSquare,
              type: 'chat-presets',
            },
          ],
        },
        {
          id: 'personal',
          title: t('Account'),
          items: [
            {
              title: t('Wallet'),
              url: '/wallet',
              icon: Wallet,
            },
            {
              title: t('Tickets'),
              url: '/profile#tickets',
              icon: LifeBuoy,
            },
            {
              title: t('Referral'),
              url: '/referral',
              icon: Gift,
            },
            // adty task 11.6: only site owners see the owner center.
            ...(isSiteOwner
              ? [
                  {
                    title: t('Site owner center'),
                    url: '/site-owner/overview',
                    activeUrls: ['/site-owner'],
                    icon: Store,
                  },
                ]
              : []),
            {
              title: t('Profile'),
              url: '/profile',
              icon: User,
            },
            {
              title: t('Security & Access'),
              url: '/security',
              icon: ShieldCheck,
            },
          ],
        },
        {
          id: 'admin',
          title: t('Administration'),
          items: [
            {
              title: t('Channels'),
              url: '/channels',
              icon: Radio,
              requiredPermission: ADTY_PERMISSIONS.CHANNEL_READ,
            },
            {
              title: t('Models'),
              url: '/models/metadata',
              icon: Box,
              requiredPermission: ADTY_PERMISSIONS.MODEL_READ,
            },
            {
              title: t('Users'),
              url: '/users',
              icon: Users,
              requiredPermission: ADTY_PERMISSIONS.USER_READ,
            },
            {
              title: t('Referral admin'),
              url: '/admin-referral',
              icon: HandCoins,
              badge: referralBadge,
              requiredPermission: ADTY_PERMISSIONS.REFERRAL_READ,
            },
            {
              title: t('Reseller sites'),
              url: '/site-admin/sites',
              activeUrls: ['/site-admin'],
              icon: Store,
              requiredPermission: ADTY_PERMISSIONS.SITE_READ,
            },
            {
              title: t('Order management'),
              url: '/recharge-audit',
              icon: ReceiptText,
              requiredPermission: ADTY_PERMISSIONS.FINANCE_READ,
            },
            {
              title: t('System Info'),
              url: '/system-info',
              icon: ServerCog,
              requiredRole: ROLE.SUPER_ADMIN,
            },
            {
              title: t('Task Plugins'),
              url: '/task-plugins',
              icon: PlugZap,
              requiredRole: ROLE.SUPER_ADMIN,
            },
            {
              title: t('System Settings'),
              url: '/system-settings/site',
              activeUrls: ['/system-settings'],
              icon: Settings,
              requiredRole: ROLE.SUPER_ADMIN,
            },
          ],
        },
      ],
    }
  }, [t, referralBadge, isSiteOwner, isStaff])
}
