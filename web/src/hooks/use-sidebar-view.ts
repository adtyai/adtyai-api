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
import { useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  getSystemSettingsNavGroups,
  pendingBadge,
  sumSidebarBadges,
} from '@/components/layout/config/system-settings.config'
import { resolveSidebarView } from '@/components/layout/lib/sidebar-view-registry'
import { useSidebarBadges } from '@/features/system-settings/sidebar-badges/hooks/use-sidebar-badges'
import type { NavGroup, ResolvedSidebarView } from '@/components/layout/types'
import {
  applyNestedAdminMenuVisibility,
  applyRootMenuVisibility,
  parseMenuVisibilityMap,
} from '@/lib/menu-visibility'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarConfig } from './use-sidebar-config'
import { useSidebarData } from './use-sidebar-data'
import { useStatus } from './use-status'

/** Sentinel key used for the root navigation in animation `key=` props */
const ROOT_VIEW_KEY = '__root'

/**
 * Resolve the active sidebar view for the current location.
 *
 * - Returns the matching nested {@link SidebarView} (with its nav
 *   groups) when the URL belongs to a registered drill-in workspace.
 * - Otherwise returns the root navigation, narrowed by:
 *     · admin-only group visibility (role-based);
 *     · `useSidebarConfig` (admin × user `sidebar_modules` overlay).
 *
 * Nested views are intentionally NOT passed through `useSidebarConfig`
 * — those filters target known dashboard URLs only, and gating is
 * already enforced at the route level (`beforeLoad` redirects).
 */
export function useSidebarView(): ResolvedSidebarView {
  const { t } = useTranslation()
  const pathname = useLocation({ select: (l) => l.pathname })
  const userRole = useAuthStore((s) => s.auth.user?.role)
  const rootSidebarData = useSidebarData()
  const configFilteredRoot = useSidebarConfig(rootSidebarData.navGroups)
  const { status } = useStatus()
  const sidebarBadges = useSidebarBadges()
  // A39: status-driven menu visibility (missing key = visible), AND-ed
  // on top of sidebar_modules. Entry-only; route guards are unchanged.
  const rawUserVisible = status?.SidebarUserVisible
  const rawAdminVisible = status?.SidebarAdminVisible
  const userVisible = useMemo(
    () => parseMenuVisibilityMap(rawUserVisible),
    [rawUserVisible]
  )
  const adminVisible = useMemo(
    () => parseMenuVisibilityMap(rawAdminVisible),
    [rawAdminVisible]
  )

  const rootNavGroups = useMemo<NavGroup[]>(() => {
    const role = userRole ?? ROLE.GUEST
    const isAdmin = role >= ROLE.ADMIN
    const settingsBadge = sumSidebarBadges(sidebarBadges)
    return applyRootMenuVisibility(
      configFilteredRoot,
      userVisible,
      adminVisible
    )
      .filter((group) => (group.id === 'admin' ? isAdmin : true))
      .map((group) => {
        const filtered = group.items.filter(
          (item) => item.requiredRole === undefined || role >= item.requiredRole
        )
        const routingBadge = pendingBadge(
          (sidebarBadges.channel_alert || 0) +
            (sidebarBadges.reconcile_alert || 0)
        )
        const items =
          group.id === 'admin' && (settingsBadge || routingBadge)
            ? filtered.map((item) => {
                const isSystemSettings =
                  ('activeUrls' in item &&
                    Array.isArray(item.activeUrls) &&
                    item.activeUrls.some(
                      (u) =>
                        typeof u === 'string' &&
                        u.startsWith('/system-settings')
                    )) ||
                  ('url' in item &&
                    typeof item.url === 'string' &&
                    item.url.startsWith('/system-settings'))
                const isSmartRouting =
                  ('items' in item &&
                    Array.isArray(item.items) &&
                    item.items.some(
                      (sub) =>
                        typeof sub.url === 'string' &&
                        sub.url.startsWith('/smart-routing')
                    )) ||
                  ('url' in item &&
                    typeof item.url === 'string' &&
                    item.url.startsWith('/smart-routing'))
                if (isSystemSettings && settingsBadge) {
                  return { ...item, badge: settingsBadge }
                }
                if (isSmartRouting && routingBadge) {
                  return { ...item, badge: routingBadge }
                }
                return item
              })
            : filtered
        const unchanged =
          items.length === group.items.length &&
          !(group.id === 'admin' && (settingsBadge || routingBadge))
        return unchanged ? group : { ...group, items }
      })
  }, [configFilteredRoot, userRole, userVisible, adminVisible, sidebarBadges])

  const view = resolveSidebarView(pathname)

  if (view) {
    return {
      key: view.id,
      view,
      navGroups:
        view.id === 'system-settings'
          ? applyNestedAdminMenuVisibility(
              getSystemSettingsNavGroups(t, sidebarBadges),
              adminVisible
            )
          : view.getNavGroups(t),
    }
  }

  return {
    key: ROOT_VIEW_KEY,
    view: null,
    navGroups: rootNavGroups,
  }
}
