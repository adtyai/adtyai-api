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

import type { MenuKeyDef, MenuVisibilitySide } from '../types'

/**
 * Known user-side menu keys.
 * Slugs derive from sidebar path last segment (accept requires `wallet`).
 * `api-tokens` is kept as an alias key alongside `keys` (/keys).
 */
export const USER_MENU_KEYS: MenuKeyDef[] = [
  { key: 'wallet', labelKey: 'Wallet' },
  { key: 'keys', labelKey: 'API Keys' },
  { key: 'api-tokens', labelKey: 'API tokens (alias)' },
  { key: 'affiliate', labelKey: 'Invite & rewards' },
  { key: 'overview', labelKey: 'Overview' },
  { key: 'dashboard', labelKey: 'Dashboard' },
  { key: 'playground', labelKey: 'Playground' },
  { key: 'usage-logs', labelKey: 'Usage Logs' },
  { key: 'profile', labelKey: 'Profile' },
  { key: 'security', labelKey: 'Security & Access' },
]

/**
 * Known admin-side menu keys (system-settings path tails + root admin nav).
 */
export const ADMIN_MENU_KEYS: MenuKeyDef[] = [
  { key: 'orders', labelKey: 'Topup orders' },
  { key: 'enterprise-pricing', labelKey: 'Enterprise pricing' },
  // Nested last-seg of /system-settings/alerts only (not operations/alerts — see menuKeysForUrl)
  { key: 'alerts', labelKey: 'Telegram alerts' },
  { key: 'chatwoot', labelKey: 'Chatwoot customer support' },
  { key: 'risk-reviews', labelKey: 'Risk reviews' },
  { key: 'aff-commissions', labelKey: 'Affiliate commissions' },
  { key: 'aff-withdrawals', labelKey: 'Affiliate withdrawals' },
  { key: 'bonus-ledger', labelKey: 'Bonus ledger' },
  { key: 'order-refunds', labelKey: 'Order refunds' },
  { key: 'channels', labelKey: 'Channels' },
  { key: 'models', labelKey: 'Models' },
  { key: 'users', labelKey: 'Users' },
  { key: 'redemption-codes', labelKey: 'Redemption Codes' },
  { key: 'system-info', labelKey: 'System Info' },
  { key: 'task-plugins', labelKey: 'Plugins' },
  { key: 'system-settings', labelKey: 'System Settings' },
]

export function knownKeysFor(side: MenuVisibilitySide): MenuKeyDef[] {
  return side === 'user' ? USER_MENU_KEYS : ADMIN_MENU_KEYS
}

export function knownKeySet(side: MenuVisibilitySide): Set<string> {
  return new Set(knownKeysFor(side).map((k) => k.key))
}
