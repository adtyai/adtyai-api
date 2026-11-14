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
*/
import { useTranslation } from 'react-i18next'

import {
  REFERRAL_ADMIN_BADGE_KINDS,
  useSidebarBadges,
} from '@/hooks/use-sidebar-badges'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { useAuthStore } from '@/stores/auth-store'

/** Pending referral work for the sidebar, from the scoped badge API. */
export function useAdminReferralBadge(): string | undefined {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const allowed = canAdmin(user, ADTY_PERMISSIONS.REFERRAL_READ)
  const sidebarBadges = useSidebarBadges()
  if (!allowed) {
    return undefined
  }
  return sidebarBadges.badge(
    REFERRAL_ADMIN_BADGE_KINDS,
    t('Sidebar badge pending work'),
    t('Sidebar badge unavailable')
  )
}
