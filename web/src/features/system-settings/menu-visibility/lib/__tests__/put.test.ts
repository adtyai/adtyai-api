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

import {
  addCustomKey,
  buildMenuVisibilityPut,
  buildSideMap,
  menuKeyVisible,
  normalizeCustomKey,
  normalizeSideDraft,
  sideIsDirty,
  viewToDrafts,
} from '../put'

describe('menuKeyVisible', () => {
  it('defaults missing key to visible', () => {
    expect(menuKeyVisible({}, 'wallet')).toBe(true)
    expect(menuKeyVisible(undefined, 'wallet')).toBe(true)
    expect(menuKeyVisible({ wallet: false }, 'wallet')).toBe(false)
    expect(menuKeyVisible({ wallet: true }, 'wallet')).toBe(true)
  })
})

describe('normalizeSideDraft', () => {
  it('fills known keys as true when server map empty', () => {
    const draft = normalizeSideDraft('user', {})
    expect(draft.values.wallet).toBe(true)
    expect(draft.values.overview).toBe(true)
    expect(draft.customKeys).toEqual([])
  })

  it('overlays server false and lifts unknown keys into customKeys', () => {
    const draft = normalizeSideDraft('user', {
      wallet: false,
      future_mod: true,
    })
    expect(draft.values.wallet).toBe(false)
    expect(draft.values.future_mod).toBe(true)
    expect(draft.customKeys).toEqual(['future_mod'])
  })

  it('admin orders false from server', () => {
    const draft = normalizeSideDraft('admin', { orders: false })
    expect(draft.values.orders).toBe(false)
    expect(draft.values.chatwoot).toBe(true)
  })
})

describe('buildSideMap / buildMenuVisibilityPut', () => {
  it('wholesale includes all draft keys (including false)', () => {
    const draft = normalizeSideDraft('user', { wallet: false })
    const map = buildSideMap(draft)
    expect(map.wallet).toBe(false)
    expect(map.keys).toBe(true)
    expect(Object.keys(map).length).toBeGreaterThan(1)
  })

  it('omits clean side; includes dirty side wholesale', () => {
    const initial = viewToDrafts({ user: {}, admin: {} })
    const current = {
      user: {
        ...initial.user,
        values: { ...initial.user.values, wallet: false },
      },
      admin: initial.admin,
    }
    const put = buildMenuVisibilityPut(current, initial)
    expect(put).not.toBeNull()
    expect(put?.user).toBeDefined()
    expect(put?.user?.wallet).toBe(false)
    expect(put).not.toHaveProperty('admin')
  })

  it('can emit empty map for a side wholesale clear', () => {
    const initial = viewToDrafts({
      user: { wallet: false },
      admin: { orders: true },
    })
    const emptyUser = { values: {}, customKeys: [] as string[] }
    const put = buildMenuVisibilityPut(
      { user: emptyUser, admin: initial.admin },
      initial
    )
    expect(put).toEqual({ user: {} })
  })

  it('returns null when nothing changed', () => {
    const drafts = viewToDrafts({ user: { wallet: false }, admin: {} })
    expect(buildMenuVisibilityPut(drafts, drafts)).toBeNull()
  })

  it('emits both sides when both dirty', () => {
    const initial = viewToDrafts({ user: {}, admin: {} })
    const current = {
      user: {
        ...initial.user,
        values: { ...initial.user.values, wallet: false },
      },
      admin: {
        ...initial.admin,
        values: { ...initial.admin.values, orders: false },
      },
    }
    const put = buildMenuVisibilityPut(current, initial)
    expect(put?.user?.wallet).toBe(false)
    expect(put?.admin?.orders).toBe(false)
  })
})

describe('sideIsDirty', () => {
  it('detects toggle', () => {
    const a = normalizeSideDraft('user', {})
    const b = {
      ...a,
      values: { ...a.values, wallet: false },
    }
    expect(sideIsDirty(b, a)).toBe(true)
    expect(sideIsDirty(a, a)).toBe(false)
  })
})

describe('normalizeCustomKey / addCustomKey', () => {
  it('rejects empty or whitespace keys', () => {
    expect(normalizeCustomKey('')).toBeNull()
    expect(normalizeCustomKey('  ')).toBeNull()
    expect(normalizeCustomKey('has space')).toBeNull()
    expect(normalizeCustomKey('future_x')).toBe('future_x')
  })

  it('adds custom key default visible', () => {
    const base = normalizeSideDraft('user', {})
    const { draft, error } = addCustomKey(base, 'user', 'new_mod')
    expect(error).toBeUndefined()
    expect(draft.values.new_mod).toBe(true)
    expect(draft.customKeys).toContain('new_mod')
  })

  it('rejects duplicate', () => {
    const base = normalizeSideDraft('user', {})
    const { error } = addCustomKey(base, 'user', 'wallet')
    expect(error).toBe('duplicate')
  })
})
