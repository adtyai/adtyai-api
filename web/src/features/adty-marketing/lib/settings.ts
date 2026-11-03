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
import type { MarketingSettings, MarketingSettingsUpdate } from '../api'

export type TextKey =
  | 'from_address'
  | 'footer_company'
  | 'footer_address'
  | 'resend_segment_prefix'
  | 'consent_version'
  | 'eu_countries'
export type NumberKey =
  | 'sync_batch_size'
  | 'sync_interval_seconds'
  | 'api_rate_per_second'
export type BoolKey =
  | 'consent_default_non_eu'
  | 'open_tracking'
  | 'click_tracking'
  | 'include_site_customers_default'
  | 'promo_enabled'

export const TEXT_KEYS: TextKey[] = [
  'from_address',
  'footer_company',
  'footer_address',
  'resend_segment_prefix',
  'consent_version',
  'eu_countries',
]
/** Marketing cannot be switched on while these are blank (CAN-SPAM footer, sender). */
export const REQUIRED_TEXT_KEYS: TextKey[] = [
  'from_address',
  'footer_company',
  'footer_address',
]
export const NUMBER_KEYS: NumberKey[] = [
  'sync_batch_size',
  'sync_interval_seconds',
  'api_rate_per_second',
]
export const BOOL_KEYS: BoolKey[] = [
  'consent_default_non_eu',
  'open_tracking',
  'click_tracking',
  'include_site_customers_default',
  'promo_enabled',
]

export type MarketingDrafts = Record<TextKey | NumberKey, string> &
  Record<BoolKey, boolean> & { enabled: boolean; webhook_secret: string }

export function toMarketingDrafts(
  settings: MarketingSettings
): MarketingDrafts {
  const drafts = {
    enabled: settings.enabled,
    webhook_secret: '',
  } as MarketingDrafts
  for (const key of TEXT_KEYS) drafts[key] = settings[key]
  for (const key of NUMBER_KEYS) drafts[key] = String(settings[key])
  for (const key of BOOL_KEYS) drafts[key] = settings[key]
  return drafts
}

/** PUT body with changed fields only; a blank secret leaves the saved one in place. */
export function buildMarketingSettingsUpdate(
  saved: MarketingSettings,
  drafts: MarketingDrafts
): MarketingSettingsUpdate {
  const body: MarketingSettingsUpdate = {}
  for (const key of TEXT_KEYS) {
    if (drafts[key].trim() !== saved[key]) body[key] = drafts[key].trim()
  }
  for (const key of NUMBER_KEYS) {
    if (Number(drafts[key]) !== saved[key]) body[key] = Number(drafts[key])
  }
  for (const key of BOOL_KEYS) {
    if (drafts[key] !== saved[key]) body[key] = drafts[key]
  }
  if (drafts.enabled !== saved.enabled) body.enabled = drafts.enabled
  return body
}
