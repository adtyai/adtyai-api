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
import type { TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'

import type { NavItem } from '../../types'
import { getSystemSettingsNavGroups } from '../system-settings.config'

const t = ((key: string) => key) as unknown as TFunction

function findByUrl(
  items: NavItem[],
  url: string
): { badge?: string } | undefined {
  for (const item of items) {
    if ('items' in item && item.items) {
      const hit = item.items.find((s) => s.url === url)
      if (hit) return hit
    }
    if ('url' in item && item.url === url) return item
  }
  return undefined
}

describe('system settings sidebar badges', () => {
  it('omits badge when counts are zero or missing', () => {
    const groups = getSystemSettingsNavGroups(t)
    const items = groups.flatMap((g) => g.items)
    expect(findByUrl(items, '/system-settings/risk-reviews')?.badge).toBeUndefined()
    expect(findByUrl(items, '/system-settings/aff-withdrawals')?.badge).toBeUndefined()
    expect(findByUrl(items, '/system-settings/orders')?.badge).toBeUndefined()
  })

  it('shows counts for risk / withdraw / topup when > 0', () => {
    const groups = getSystemSettingsNavGroups(t, {
      risk_pending: 3,
      withdraw_pending: 12,
      topup_pending: 100,
    })
    const items = groups.flatMap((g) => g.items)
    expect(findByUrl(items, '/system-settings/risk-reviews')?.badge).toBe('3')
    expect(findByUrl(items, '/system-settings/aff-withdrawals')?.badge).toBe('12')
    expect(findByUrl(items, '/system-settings/orders')?.badge).toBe('99+')
  })
})
