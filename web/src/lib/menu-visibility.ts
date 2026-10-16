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
import type { NavGroup, NavItem } from '@/components/layout/types'

/**
 * A39 menu visibility (status.SidebarUserVisible / SidebarAdminVisible).
 *
 * Map of menu key → visible. Missing map / missing key / non-false value =
 * visible. Only hides sidebar entries; routes & permission guards untouched.
 * Combined with sidebar_modules by AND (applied after useSidebarConfig).
 */
export type MenuVisibilityMap = Record<string, boolean>

const KEY_ALIASES: Record<string, string[]> = {
  keys: ['api-tokens'],
  'task-plugins': ['plugins'],
}

export function parseMenuVisibilityMap(raw: unknown): MenuVisibilityMap {
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
  const out: MenuVisibilityMap = {}
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    if (typeof v === 'boolean') out[k] = v
  }
  return out
}

/**
 * Candidate menu keys for a URL. Root nav: every path segment
 * (e.g. /usage-logs/common → usage-logs, common). Nested views
 * (system-settings drill-in): last segment only, so hiding the
 * `system-settings` key does not wipe the whole drill-in list.
 */
export function menuKeysForUrl(
  url: string,
  mode: 'root' | 'nested' = 'root'
): string[] {
  const path = url.split(/[?#]/)[0]
  const segments = path.split('/').filter(Boolean)
  const keys = new Set<string>()
  if (mode === 'nested') {
    // /system-settings/operations/alerts (Monitoring) must not share key
    // `alerts` with /system-settings/alerts (Telegram).
    if (
      segments.length >= 2 &&
      segments.at(-1) === 'alerts' &&
      segments.at(-2) === 'operations'
    ) {
      keys.add('operations-alerts')
      return [...keys]
    }
    const last = segments.slice(-1)
    for (const seg of last) {
      keys.add(seg)
      for (const alias of KEY_ALIASES[seg] ?? []) keys.add(alias)
    }
    return [...keys]
  }
  for (const seg of segments) {
    keys.add(seg)
    for (const alias of KEY_ALIASES[seg] ?? []) keys.add(alias)
  }
  return [...keys]
}

export function isMenuUrlVisible(
  url: string | undefined,
  map: MenuVisibilityMap,
  mode: 'root' | 'nested' = 'root'
): boolean {
  if (!url) return true
  return !menuKeysForUrl(String(url), mode).some((k) => map[k] === false)
}

function filterItems(
  items: NavItem[],
  map: MenuVisibilityMap,
  mode: 'root' | 'nested'
): NavItem[] {
  const out: NavItem[] = []
  for (const item of items) {
    if ('items' in item && item.items) {
      const sub = item.items.filter((s) =>
        isMenuUrlVisible(s.url as string, map, mode)
      )
      if (sub.length > 0) {
        out.push(
          sub.length === item.items.length ? item : { ...item, items: sub }
        )
      }
      continue
    }
    if ('url' in item && item.url) {
      if (isMenuUrlVisible(item.url as string, map, mode)) out.push(item)
      continue
    }
    out.push(item)
  }
  return out
}

/**
 * Root sidebar: group `admin` uses the admin map, every other group the
 * user map. Groups emptied by the filter are dropped.
 */
export function applyRootMenuVisibility(
  groups: NavGroup[],
  userMap: MenuVisibilityMap,
  adminMap: MenuVisibilityMap
): NavGroup[] {
  return groups
    .map((g) => {
      const map = g.id === 'admin' ? adminMap : userMap
      const items = filterItems(g.items, map, 'root')
      return items.length === g.items.length &&
        items.every((it, i) => it === g.items[i])
        ? g
        : { ...g, items }
    })
    .filter((g) => g.items.length > 0)
}

/** Nested admin drill-in views (system-settings): admin map only. */
export function applyNestedAdminMenuVisibility(
  groups: NavGroup[],
  adminMap: MenuVisibilityMap
): NavGroup[] {
  return groups
    .map((g) => ({ ...g, items: filterItems(g.items, adminMap, 'nested') }))
    .filter((g) => g.items.length > 0)
}
