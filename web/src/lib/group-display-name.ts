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
/**
 * A08 group display names (status.GroupDisplayNames: code → display name).
 * Display-only: stored / submitted values remain the group code.
 */
export type GroupDisplayNameMap = Record<string, string>

export function parseGroupDisplayNames(raw: unknown): GroupDisplayNameMap {
  let value: unknown = raw
  if (typeof value === 'string') {
    if (!value.trim()) return {}
    try {
      value = JSON.parse(value)
    } catch {
      return {}
    }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const out: GroupDisplayNameMap = {}
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    if (typeof v === 'string' && v.trim()) out[k] = v.trim()
  }
  return out
}

/** Display name for a code; falls back to the code when no meta. */
export function resolveGroupDisplayName(
  code: string | null | undefined,
  map: GroupDisplayNameMap
): string {
  const c = (code ?? '').trim()
  if (!c) return ''
  return map[c] ?? c
}
