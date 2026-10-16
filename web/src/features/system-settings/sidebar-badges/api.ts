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

import { api } from '@/lib/api'

import {
  EMPTY_SIDEBAR_BADGES,
  type SidebarBadges,
  type SidebarBadgesResponse,
} from './types'

function asCount(value: unknown): number {
  const n = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(n) || n <= 0) return 0
  return Math.floor(n)
}

/**
 * Admin sidebar pending counts.
 * Missing endpoint / auth / network → silent zeros (no toast).
 */
export async function getSidebarBadges(): Promise<SidebarBadges> {
  try {
    const res = await api.get<SidebarBadgesResponse>(
      '/api/adty/admin/sidebar_badges',
      {
        skipErrorHandler: true,
        validateStatus: () => true,
      }
    )
    if (res.status < 200 || res.status >= 300) {
      return EMPTY_SIDEBAR_BADGES
    }
    const data = res.data?.data
    if (!data || typeof data !== 'object') {
      return EMPTY_SIDEBAR_BADGES
    }
    return {
      risk_pending: asCount(data.risk_pending),
      withdraw_pending: asCount(data.withdraw_pending),
      topup_pending: asCount(data.topup_pending),
    }
  } catch {
    return EMPTY_SIDEBAR_BADGES
  }
}
