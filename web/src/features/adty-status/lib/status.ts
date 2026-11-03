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
import type { UptimeGroupResult } from '@/features/dashboard/types'

/** Uptime Kuma heartbeat status: 0 down, 1 up, 2 pending, 3 maintenance. */
export type MonitorState = 'up' | 'down' | 'pending' | 'maintenance' | 'unknown'

/** A monitor as the public page shows it: no Uptime Kuma group name. */
export type PublicMonitor = {
  name: string
  state: MonitorState
  /** Share of the last 24 hours the monitor was up, 0 to 1. */
  uptime: number
}

export type PublicStatusGroup = {
  /** Stable id for the heading anchor. */
  id: number
  name: string
  monitors: PublicMonitor[]
}

const STATES: Record<number, MonitorState> = {
  0: 'down',
  1: 'up',
  2: 'pending',
  3: 'maintenance',
}

/**
 * The categories and monitors the public status page may show (task 11.10).
 * Keeps only the category set in the Uptime Kuma settings and each
 * monitor's name, state and uptime. The `group` of a monitor is Uptime
 * Kuma's own public group, which operators often name after channels or
 * upstreams, so it is dropped. Empty categories are left out.
 */
export function toPublicStatusGroups(
  groups: UptimeGroupResult[] | undefined
): PublicStatusGroup[] {
  return (groups ?? []).flatMap((group, index) => {
    const monitors = (group.monitors ?? []).map((monitor) => ({
      name: monitor.name,
      state: STATES[monitor.status] ?? 'unknown',
      uptime: Math.min(Math.max(Number(monitor.uptime) || 0, 0), 1),
    }))
    if (monitors.length === 0) return []
    return [{ id: index, name: group.categoryName, monitors }]
  })
}

/** True when no monitor is down, pending or of unknown state. */
export function isAllOperational(groups: PublicStatusGroup[]): boolean {
  return groups.every((group) =>
    group.monitors.every(
      (monitor) => monitor.state === 'up' || monitor.state === 'maintenance'
    )
  )
}
