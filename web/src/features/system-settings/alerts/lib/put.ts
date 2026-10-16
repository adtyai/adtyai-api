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

import type {
  AlertsConfigView,
  AlertsFormValues,
  AlertsPutInput,
} from '../types'

export const ALERTS_FORM_DEFAULTS: AlertsFormValues = {
  bot_token: '',
  urgent_chat_id: '',
  pending_chat_id: '',
  urgent_route_enabled: true,
  pending_route_enabled: true,
  kind_enabled: {},
  merge_minutes: 10,
}

export function formatTokenFingerprint(fp: string | undefined | null): string {
  if (!fp) return '-'
  return fp
}

export function configToForm(view: AlertsConfigView): AlertsFormValues {
  return {
    bot_token: '',
    urgent_chat_id: view.urgent_chat_id ?? '',
    pending_chat_id: view.pending_chat_id ?? '',
    urgent_route_enabled: Boolean(view.urgent_route_enabled),
    pending_route_enabled: Boolean(view.pending_route_enabled),
    kind_enabled: view.kind_enabled ? { ...view.kind_enabled } : {},
    merge_minutes:
      typeof view.merge_minutes === 'number' &&
      !Number.isNaN(view.merge_minutes)
        ? view.merge_minutes
        : 10,
  }
}

function kindMapsEqual(
  a: Record<string, boolean>,
  b: Record<string, boolean>
): boolean {
  const keys = new Set([...Object.keys(a), ...Object.keys(b)])
  for (const k of keys) {
    if (Boolean(a[k]) !== Boolean(b[k])) return false
  }
  return true
}

/**
 * Build PUT body with pointer semantics: only include fields that changed.
 * Empty / whitespace-only bot_token is omitted (leave blank = do not overwrite).
 * Returns null when there is nothing to send.
 */
export function buildAlertsPut(
  next: AlertsFormValues,
  initial: AlertsFormValues
): AlertsPutInput | null {
  const body: AlertsPutInput = {}

  const token = next.bot_token.trim()
  if (token) {
    body.bot_token = token
  }

  const urgentChat = next.urgent_chat_id.trim()
  const initialUrgent = initial.urgent_chat_id.trim()
  if (urgentChat !== initialUrgent) {
    body.urgent_chat_id = urgentChat
  }

  const pendingChat = next.pending_chat_id.trim()
  const initialPending = initial.pending_chat_id.trim()
  if (pendingChat !== initialPending) {
    body.pending_chat_id = pendingChat
  }

  if (next.urgent_route_enabled !== initial.urgent_route_enabled) {
    body.urgent_route_enabled = next.urgent_route_enabled
  }

  if (next.pending_route_enabled !== initial.pending_route_enabled) {
    body.pending_route_enabled = next.pending_route_enabled
  }

  if (!kindMapsEqual(next.kind_enabled, initial.kind_enabled)) {
    body.kind_enabled = { ...next.kind_enabled }
  }

  if (next.merge_minutes !== initial.merge_minutes) {
    body.merge_minutes = next.merge_minutes
  }

  return Object.keys(body).length === 0 ? null : body
}

/**
 * Pending Telegram channel ready for B13 daily report (no token plaintext).
 * Requires secret_set, non-empty pending_chat_id, and pending route enabled.
 */
export function isTelegramPendingConfigured(
  view:
    | Pick<
        AlertsConfigView,
        'bot_token' | 'pending_chat_id' | 'pending_route_enabled'
      >
    | null
    | undefined
): boolean {
  if (!view) return false
  return (
    Boolean(view.bot_token?.secret_set) &&
    Boolean(view.pending_chat_id?.trim()) &&
    Boolean(view.pending_route_enabled)
  )
}

/** Sorted kind keys for stable UI order. */
export function sortedKindKeys(kindEnabled: Record<string, boolean>): string[] {
  return Object.keys(kindEnabled).sort((a, b) => a.localeCompare(b))
}

/** Sorted threshold keys for readonly table. */
export function sortedThresholdKeys(
  thresholds: Record<string, string>
): string[] {
  return Object.keys(thresholds).sort((a, b) => a.localeCompare(b))
}
