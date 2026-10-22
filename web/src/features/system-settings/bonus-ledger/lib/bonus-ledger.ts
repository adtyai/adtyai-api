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

import {
  clampLedgerLimit,
  clampLedgerOffset,
} from '@/features/wallet/ledger/lib/ledger'

import type { AdminBonusLedgerParams } from '../types'

/** Kinds that can appear on the bonus account. This page never queries paid or aff. */
export const BONUS_LEDGER_KIND_OPTIONS = [
  'topup_tier_bonus',
  'topup_first_bonus',
  'api_consume_hourly',
  'topup_refund_bonus_claw',
  'balance_adjust',
  'redemption_bonus',
  'signup_invite_bonus',
  'checkin_bonus',
] as const

export {
  DEFAULT_LEDGER_LIMIT,
  LEDGER_KIND_OPTIONS,
  clampLedgerLimit,
  clampLedgerOffset,
  datetimeLocalToUnix,
  formatLedgerTime,
  formatSignedQuota,
  ledgerKindLabelKey,
  ledgerNote,
  parseUserIdFilter,
} from '@/features/wallet/ledger/lib/ledger'

/**
 * Build query for GET /api/admin/wallet/bonus-ledger.
 * Backend forces account=bonus; do not send account.
 */
export function buildAdminBonusLedgerQuery(
  params: AdminBonusLedgerParams
): Record<string, string | number> {
  const out: Record<string, string | number> = {
    limit: clampLedgerLimit(params.limit),
    offset: clampLedgerOffset(params.offset),
  }
  if (params.user_id != null && params.user_id > 0) {
    out.user_id = Math.floor(params.user_id)
  }
  const kind = (params.kind || '').trim()
  if (kind) out.kind = kind
  if (params.created_from != null && params.created_from !== '') {
    out.created_from = params.created_from
  }
  if (params.created_to != null && params.created_to !== '') {
    out.created_to = params.created_to
  }
  return out
}
