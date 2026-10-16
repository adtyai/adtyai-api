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

import { useIsAdmin } from '@/hooks/use-admin'

import { getSidebarBadges } from '../api'
import { EMPTY_SIDEBAR_BADGES, type SidebarBadges } from '../types'

export const SIDEBAR_BADGES_QUERY_KEY = ['adty-sidebar-badges'] as const

const POLL_MS = 30_000

/**
 * Poll sidebar pending counts for admins (root「系统设置」+ 钻入侧栏都要用).
 * Failures resolve to zeros (getSidebarBadges never throws).
 */
export function useSidebarBadges(): SidebarBadges {
  const isAdmin = useIsAdmin()
  const enabled = isAdmin

  const { data } = useQuery({
    queryKey: SIDEBAR_BADGES_QUERY_KEY,
    queryFn: () => getSidebarBadges(),
    enabled,
    refetchInterval: enabled ? POLL_MS : false,
    staleTime: 15_000,
    retry: false,
  })

  return data ?? EMPTY_SIDEBAR_BADGES
}
