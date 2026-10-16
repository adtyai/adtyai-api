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

/** Counts from GET /api/adty/admin/sidebar_badges */
export type SidebarBadges = {
  risk_pending: number
  withdraw_pending: number
  topup_pending: number
  /** Smart routing alerts: breaker / balance / fake model / cost guard / reconcile. */
  channel_alert?: number
  reconcile_alert?: number
}

export type SidebarBadgesResponse = {
  success: boolean
  message?: string
  data?: SidebarBadges
}

export const EMPTY_SIDEBAR_BADGES: SidebarBadges = {
  risk_pending: 0,
  withdraw_pending: 0,
  topup_pending: 0,
  channel_alert: 0,
  reconcile_alert: 0,
}
