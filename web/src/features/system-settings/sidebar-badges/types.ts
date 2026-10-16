/** Optional smart-routing alert breakdown (sum should equal channel_alert). */
export type ChannelAlertBreakdown = {
  breaker?: number
  banned?: number
  balance?: number
  fake_model?: number
  cost_guard?: number
}

/** Counts from GET /api/adty/admin/sidebar_badges */
export type SidebarBadges = {
  risk_pending: number
  withdraw_pending: number
  topup_pending: number
  /** Smart routing alerts: breaker / balance / fake model / cost guard / reconcile. */
  channel_alert?: number
  reconcile_alert?: number
  /** When backend has not split yet, fields stay undefined → UI shows empty placeholders. */
  channel_alert_breakdown?: ChannelAlertBreakdown
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