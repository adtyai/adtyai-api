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
import { type QueryClient, useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

export const sidebarBadgeKeys = {
  all: ['adty-sidebar-badges'] as const,
}

export type SidebarBadgeCounts = Record<string, number>

export const CONFIRMED_SIDEBAR_BADGE_KINDS = [
  'notification_unread',
  'referral_partners',
  'referral_withdrawals_pending',
  'referral_withdrawals_approved',
  'site_withdrawals_pending',
  'invite_withdrawals_pending',
  'site_invite_commissions_pending_coverage',
  'referral_commissions_frozen',
  'fund_anomalies_open',
  'staff_balance_pending',
  'tickets_pending',
  'tickets_waiting_user',
  'payment_orphans_pending_review',
] as const

export const UNCONFIRMED_SIDEBAR_BADGE_KINDS = [
  'topup_orders_pending',
  'payment_exceptions_other',
  'team_notification_unread',
  'site_applications_pending',
  'site_invite_commissions_frozen',
  'site_withdrawals_paying',
  'manual_refunds_submitted',
] as const

export const REFERRAL_ADMIN_BADGE_KINDS = [
  'referral_partners',
  'referral_withdrawals_pending',
  'referral_withdrawals_approved',
  'referral_commissions_frozen',
] as const

export const ORDER_MANAGEMENT_BADGE_KINDS = [
  'payment_orphans_pending_review',
] as const

export const RESELLER_SITES_BADGE_KINDS = [
  'site_withdrawals_pending',
  'fund_anomalies_open',
  'staff_balance_pending',
] as const

export const SITE_OWNER_BADGE_KINDS = [
  'invite_withdrawals_pending',
  'site_invite_commissions_pending_coverage',
  'staff_balance_pending',
] as const

export const TICKET_BADGE_KINDS = [
  'tickets_pending',
  'tickets_waiting_user',
] as const

export const STAFF_WORKSPACE_BADGE_KINDS = ['tickets_pending'] as const

/** Existing referral sidebar refetch. Not a newly signed official interval. */
export const SIDEBAR_BADGE_EXISTING_REFETCH_MS = 5 * 60_000

export async function getSidebarBadges(): Promise<SidebarBadgeCounts> {
  const res = await api.get<ApiResponse<{ counts: SidebarBadgeCounts }>>(
    '/api/adty/badges'
  )
  return requireServerSuccess(res.data).data.counts ?? {}
}

export function invalidateSidebarBadges(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: sidebarBadgeKeys.all })
}

/**
 * Build the sidebar cue for one menu item. Different duties are never summed.
 * A failed or stale query must not be shown as a successful number.
 */
export function sidebarBadgeText(
  counts: SidebarBadgeCounts | undefined,
  kinds: readonly string[],
  options: { failed?: boolean; mixedLabel: string; failedLabel: string }
): string | undefined {
  if (options.failed) {
    return options.failedLabel
  }
  if (!counts) {
    return undefined
  }
  const values = kinds
    .map((kind) => counts[kind])
    .filter((value): value is number => typeof value === 'number' && value > 0)
  if (values.length === 0) {
    return undefined
  }
  if (values.length === 1) {
    return String(values[0])
  }
  return options.mixedLabel
}

export function useSidebarBadges(): {
  counts: SidebarBadgeCounts | undefined
  failed: boolean
  badge: (kinds: readonly string[], mixedLabel: string, failedLabel: string) => string | undefined
} {
  const userID = useAuthStore((state) => state.auth.user?.id ?? 0)
  const query = useQuery({
    queryKey: sidebarBadgeKeys.all,
    queryFn: getSidebarBadges,
    enabled: userID > 0,
    // Keep the existing referral sidebar cadence so that badge does not go
    // staler. The interval itself is not a newly signed official rule.
    refetchInterval: SIDEBAR_BADGE_EXISTING_REFETCH_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    retry: false,
    meta: { errorToast: false },
  })
  // LIST-012: after a failed refetch, cached rows must not be shown as success.
  const failed = query.isError
  const counts = failed || !query.isSuccess ? undefined : query.data
  return {
    counts,
    failed,
    badge: (kinds, mixedLabel, failedLabel) =>
      sidebarBadgeText(counts, kinds, { failed, mixedLabel, failedLabel }),
  }
}
