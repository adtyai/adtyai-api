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

import { getSystemSettingsNavGroups } from '@/components/layout/config/system-settings.config'
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
    return applyRootMenuVisibility(
      configFilteredRoot,
      userVisible,
      adminVisible
    )
      .filter((group) => (group.id === 'admin' ? isAdmin : true))
      .map((group) => {
        const items = group.items.filter(
          (item) => item.requiredRole === undefined || role >= item.requiredRole
        )
        return items.length === group.items.length ? group : { ...group, items }
      })
  }, [configFilteredRoot, userRole, userVisible, adminVisible])

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
