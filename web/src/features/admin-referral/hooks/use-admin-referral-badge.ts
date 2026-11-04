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
import { useQuery } from '@tanstack/react-query'

import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { adminReferralKeys, getReferralBadges } from '../api'

/** Pending referral work for the sidebar: partner applications plus withdrawals to review or pay. */
export function useAdminReferralBadge(): string | undefined {
  const user = useAuthStore((state) => state.auth.user)
  const allowed = canAdmin(user, ADTY_PERMISSIONS.REFERRAL_READ)
  const query = useQuery({
    queryKey: adminReferralKeys.badges,
    queryFn: getReferralBadges,
    enabled: allowed,
    // A badge is not worth redrawing the sidebar every minute, and a
    // hidden tab should not keep asking.
    refetchInterval: 5 * 60_000,
    refetchIntervalInBackground: false,
    meta: { errorToast: false },
  })
  const total = query.data?.total ?? 0
  return allowed && total > 0 ? String(total) : undefined
}
