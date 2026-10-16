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

import { ROLE } from '@/lib/roles'

import { SYSTEM_SETTINGS_VIEW } from '../system-settings.config'

const t = ((key: string) => key) as unknown as TFunction

describe('system-settings Root-only nav (A39/A08) and no A30 audit', () => {
  it('menu-visibility and groups require SUPER_ADMIN; audit entry is gone', () => {
    const groups = SYSTEM_SETTINGS_VIEW.getNavGroups(t)
    const items = groups.flatMap((g) => g.items)
    const menu = items.find((i) => i.url === '/system-settings/menu-visibility')
    const group = items.find((i) => i.url === '/system-settings/groups')
    const audit = items.find((i) => i.url === '/system-settings/audit')
    const admins = items.find((i) => i.url === '/system-settings/admins')

    expect(menu?.requiredRole).toBe(ROLE.SUPER_ADMIN)
    expect(group?.requiredRole).toBe(ROLE.SUPER_ADMIN)
    expect(admins?.requiredRole).toBe(ROLE.SUPER_ADMIN)
    expect(audit).toBeUndefined()
  })
})
