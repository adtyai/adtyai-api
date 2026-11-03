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
import type { ReferralSettings, ReferralSettingsUpdate } from '../api'

export type MoneyKey = 'default_rate' | 'min_withdraw_usd' | 'usdt_withdraw_fee'
export type RuleKey =
  | 'cookie_ttl_days'
  | 'freeze_days'
  | 'cancel_window_minutes'
  | 'self_invite_window_days'
  | 'batch_register_threshold'
  | 'batch_register_window_hours'

export const MONEY_KEYS: MoneyKey[] = [
  'default_rate',
  'min_withdraw_usd',
  'usdt_withdraw_fee',
]
export const RULE_KEYS: RuleKey[] = [
  'cookie_ttl_days',
  'freeze_days',
  'cancel_window_minutes',
  'self_invite_window_days',
  'batch_register_threshold',
  'batch_register_window_hours',
]

export type Drafts = Record<MoneyKey | RuleKey, string> & { cny_rate: string }

export function toDrafts(settings: ReferralSettings): Drafts {
  const drafts = { cny_rate: settings.withdraw_cny_rate } as Drafts
  for (const key of MONEY_KEYS) {
    const value = settings[key]
    drafts[key] = value === null ? '' : String(value)
  }
  for (const key of RULE_KEYS) drafts[key] = String(settings[key])
  return drafts
}

/** Builds the PUT body: changed fields only; an emptied money field is sent in clear. */
export function buildSettingsUpdate(
  saved: ReferralSettings,
  drafts: Drafts,
  enabled: boolean
): ReferralSettingsUpdate {
  const body: ReferralSettingsUpdate = {}
  const clear: MoneyKey[] = []
  for (const key of MONEY_KEYS) {
    const draft = drafts[key].trim()
    if (draft === '') {
      if (saved[key] !== null) clear.push(key)
    } else if (Number(draft) !== saved[key]) {
      body[key] = Number(draft)
    }
  }
  for (const key of RULE_KEYS) {
    if (Number(drafts[key]) !== saved[key]) body[key] = Number(drafts[key])
  }
  if (clear.length > 0) body.clear = clear
  if (enabled !== saved.enabled) body.enabled = enabled
  if (drafts.cny_rate.trim() !== saved.withdraw_cny_rate) {
    body.withdraw_cny_rate = drafts.cny_rate.trim()
  }
  return body
}
