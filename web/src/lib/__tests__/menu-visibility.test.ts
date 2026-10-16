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
import { describe, expect, it } from 'vitest'

import type { NavGroup } from '@/components/layout/types'

import {
  applyNestedAdminMenuVisibility,
  applyRootMenuVisibility,
  isMenuUrlVisible,
  parseMenuVisibilityMap,
} from '../menu-visibility'

const groups: NavGroup[] = [
  {
    id: 'personal',
    title: 'p',
    items: [
      { title: 'Wallet', url: '/wallet' },
      { title: 'Keys', url: '/keys' },
      { title: 'Logs', items: [{ title: 'c', url: '/usage-logs/common' }] },
    ],
  },
  {
    id: 'admin',
    title: 'a',
    items: [
      { title: 'Users', url: '/users' },
      { title: 'Wallet-ish', url: '/wallet' },
    ],
  },
]

describe('menu visibility (A39)', () => {
  it('missing map / missing key / bad input = visible', () => {
    expect(parseMenuVisibilityMap(undefined)).toEqual({})
    expect(parseMenuVisibilityMap('not json')).toEqual({})
    expect(parseMenuVisibilityMap({ wallet: 'no' })).toEqual({})
    expect(isMenuUrlVisible('/wallet', {})).toBe(true)
    expect(applyRootMenuVisibility(groups, {}, {})).toEqual(groups)
  })

  it('false hides only the matching entry, true keeps it', () => {
    const out = applyRootMenuVisibility(
      groups,
      parseMenuVisibilityMap('{"wallet":false,"keys":true}'),
      {}
    )
    expect(out[0].items.map((i) => i.title)).toEqual(['Keys', 'Logs'])
    // user map never touches admin group
    expect(out[1].items.map((i) => i.title)).toEqual(['Users', 'Wallet-ish'])
  })

  it('api-tokens alias hides /keys; collapsibles drop empty', () => {
    const out = applyRootMenuVisibility(
      groups,
      { 'api-tokens': false, 'usage-logs': false },
      {}
    )
    expect(out[0].items.map((i) => i.title)).toEqual(['Wallet'])
  })

  it('admin map only affects admin group; empty groups dropped', () => {
    const out = applyRootMenuVisibility(
      groups,
      {},
      { users: false, wallet: false }
    )
    expect(out.map((g) => g.id)).toEqual(['personal'])
    expect(out[0].items).toHaveLength(3)
  })

  it('nested system-settings uses last segment only', () => {
    const nested: NavGroup[] = [
      {
        title: 's',
        items: [
          { title: 'Groups', url: '/system-settings/groups' },
          { title: 'Orders', url: '/system-settings/orders' },
        ],
      },
    ]
    expect(
      applyNestedAdminMenuVisibility(nested, { 'system-settings': false })[0]
        .items
    ).toHaveLength(2)
    expect(
      applyNestedAdminMenuVisibility(nested, { groups: false })[0].items.map(
        (i) => i.title
      )
    ).toEqual(['Orders'])
  })

  it('nested alerts: Telegram vs operations Monitoring do not share key', () => {
    const nested: NavGroup[] = [
      {
        title: 's',
        items: [
          { title: 'Telegram', url: '/system-settings/alerts' },
          { title: 'Monitoring', url: '/system-settings/operations/alerts' },
          { title: 'Chatwoot', url: '/system-settings/chatwoot' },
        ],
      },
    ]
    const hideTelegram = applyNestedAdminMenuVisibility(nested, {
      alerts: false,
    })
    expect(hideTelegram[0].items.map((i) => i.title)).toEqual([
      'Monitoring',
      'Chatwoot',
    ])
    const hideMonitoring = applyNestedAdminMenuVisibility(nested, {
      'operations-alerts': false,
    })
    expect(hideMonitoring[0].items.map((i) => i.title)).toEqual([
      'Telegram',
      'Chatwoot',
    ])
  })
})
