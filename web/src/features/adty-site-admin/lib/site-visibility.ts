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
import { VISIBILITY_KEYS, type VisibilityKey } from './settings'

export type SiteVisibilityDraft = Record<VisibilityKey, string>

export function validSiteVisibility(value: string | undefined): boolean {
  if (!value?.trim()) return true
  try {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    return Object.entries(parsed).every(([key, setting]) => {
      if (!VISIBILITY_KEYS.includes(key as VisibilityKey)) return false
      return (
        key === 'customer_email' ? ['masked', 'full'] : ['on', 'off']
      ).includes(setting as string)
    })
  } catch {
    return false
  }
}

/** Empty values inherit the global policy; only explicit site overrides persist. */
export function readSiteVisibility(
  value: string | undefined
): SiteVisibilityDraft {
  let saved: Record<string, unknown> = {}
  try {
    const parsed: unknown = JSON.parse(value || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      saved = parsed as Record<string, unknown>
    }
  } catch {
    // The editor separately reports invalid persisted data and disables saves.
  }
  return Object.fromEntries(
    VISIBILITY_KEYS.map((key) => {
      const allowed =
        key === 'customer_email' ? ['masked', 'full'] : ['on', 'off']
      const setting = saved[key]
      return [
        key,
        typeof setting === 'string' && allowed.includes(setting) ? setting : '',
      ]
    })
  ) as SiteVisibilityDraft
}

export function serializeSiteVisibility(draft: SiteVisibilityDraft): string {
  return JSON.stringify(
    Object.fromEntries(
      VISIBILITY_KEYS.filter((key) => draft[key] !== '').map((key) => [
        key,
        draft[key],
      ])
    )
  )
}
