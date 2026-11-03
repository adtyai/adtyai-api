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
/** Columns the site list asks the server to sort (GET /api/adty/admin/sites). */
export type SiteSortKey =
  | 'customers'
  | 'topup'
  | 'revenue'
  | 'earning'
  | 'growth'
export type SortDirection = 'asc' | 'desc'
export type SiteSort = { key: SiteSortKey; direction: SortDirection }

export const DEFAULT_SITE_SORT: SiteSort = {
  key: 'revenue',
  direction: 'desc',
}

/** Clicking the sorted column flips it; another column starts descending. */
export function nextSiteSort(current: SiteSort, key: SiteSortKey): SiteSort {
  if (current.key !== key) return { key, direction: 'desc' }
  return { key, direction: current.direction === 'desc' ? 'asc' : 'desc' }
}

/** The notice in a site's brand JSON; empty when there is none or the JSON is bad. */
export function siteNotice(brand: string | undefined): string {
  if (!brand) return ''
  try {
    const parsed: unknown = JSON.parse(brand)
    if (parsed && typeof parsed === 'object' && 'notice' in parsed) {
      const notice = (parsed as { notice?: unknown }).notice
      return typeof notice === 'string' ? notice.trim() : ''
    }
  } catch {
    return ''
  }
  return ''
}
