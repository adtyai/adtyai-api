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
  ADTY_PERMISSIONS,
  canAdmin,
  type AdtyPermission,
} from '@/lib/adty-permissions'
import { ROLE } from '@/lib/roles'
import type { AuthUser } from '@/stores/auth-store'

/** Sections of /site-admin/$section, each with the permission it needs (task 11.9). */
export const SITE_ADMIN_SECTIONS = [
  { id: 'sites', permission: ADTY_PERMISSIONS.SITE_READ },
  { id: 'overview', permission: ADTY_PERMISSIONS.SITE_READ },
  { id: 'reviews', permission: ADTY_PERMISSIONS.SITE_REVIEW },
  { id: 'settings', permission: ADTY_PERMISSIONS.SITE_SETTINGS_READ },
  // Withdrawal review and recharge codes are Root-only on the server.
  { id: 'withdrawals', permission: ADTY_PERMISSIONS.SITE_READ, rootOnly: true },
  { id: 'vouchers', permission: ADTY_PERMISSIONS.SITE_READ, rootOnly: true },
] as const satisfies ReadonlyArray<{
  id: string
  permission: AdtyPermission
  rootOnly?: boolean
}>

export type SiteAdminSectionId = (typeof SITE_ADMIN_SECTIONS)[number]['id']

export const SITE_ADMIN_DEFAULT_SECTION: SiteAdminSectionId = 'sites'

export function isSiteAdminSection(value: string): value is SiteAdminSectionId {
  return SITE_ADMIN_SECTIONS.some((section) => section.id === value)
}

/** Each section follows its explicitly granted reseller permission; Root-only sections also need Root. */
export function canOpenSiteAdminSection(
  user: AuthUser | null | undefined,
  section: (typeof SITE_ADMIN_SECTIONS)[number]
): boolean {
  if ('rootOnly' in section && section.rootOnly) {
    return user?.role === ROLE.SUPER_ADMIN
  }
  return canAdmin(user, section.permission)
}
