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

import type { GroupMetaView } from '../../types'
import {
  GROUP_CODE_RE,
  buildGroupPut,
  buildGroupTableSave,
  groupApiErrorToastKey,
  groupRatioDraftValue,
  isValidGroupCode,
  validateCreateInput,
  viewToDraft,
} from '../form'

const base: GroupMetaView = {
  code: 'default',
  display_name: 'Default',
  sort: 10,
  has_meta: true,
}

describe('validateCreateInput', () => {
  it('rejects auto even though the code shape is valid', () => {
    expect(
      validateCreateInput({ code: 'auto', display_name: 'Auto', sort: '0' })
    ).toEqual({ ok: false, error: 'invalid_code' })
  })
})

describe('groupRatioDraftValue', () => {
  it('rejects an empty ratio instead of treating it as zero', () => {
    expect(groupRatioDraftValue('')).toBeNull()
    expect(groupRatioDraftValue('   ')).toBeNull()
    expect(groupRatioDraftValue('abc')).toBeNull()
    expect(groupRatioDraftValue('-1')).toBeNull()
    expect(groupRatioDraftValue('0')).toBe(0)
    expect(groupRatioDraftValue('1.5')).toBe(1.5)
  })
})

describe('buildGroupTableSave', () => {
  it('does not turn a non-billing code into a ratio row', () => {
    const saved = buildGroupTableSave({
      ratio: { vip: 2 },
      usable: { vip: 'VIP', channelonly: 'Channel' },
      drafts: {
        vip: { ratio: '1.5', isPublic: true, displayName: 'VIP' },
        channelonly: { ratio: '0', isPublic: true, displayName: 'Channel' },
      },
      currentDisplayNames: { vip: 'VIP' },
    })
    expect(saved).toEqual({
      ok: true,
      changed: true,
      ratio: { vip: 1.5 },
      usable: { vip: 'VIP' },
      displayNames: {},
    })
  })

  it('refuses the whole save when a billing row has no draft', () => {
    expect(
      buildGroupTableSave({
        ratio: { default: 1, vip: 2 },
        usable: { default: 'Default' },
        drafts: {
          default: { ratio: '1', isPublic: true, displayName: 'Default' },
        },
        currentDisplayNames: {},
      })
    ).toEqual({ ok: false, error: 'draft_missing' })
  })

  it('keeps an explicit zero and does not write when nothing changed', () => {
    expect(
      buildGroupTableSave({
        ratio: { vip: 0, auto: 1 },
        usable: { vip: 'VIP' },
        drafts: {
          vip: { ratio: '0', isPublic: false, displayName: 'VIP' },
          auto: { ratio: '', isPublic: true, displayName: 'Auto' },
        },
        currentDisplayNames: { vip: 'VIP' },
      })
    ).toEqual({
      ok: true,
      changed: true,
      ratio: { vip: 0, auto: 1 },
      usable: {},
      displayNames: {},
    })
  })
})

describe('GROUP_CODE_RE / isValidGroupCode', () => {
  it('accepts a-z0-9_- length 2–32', () => {
    expect(isValidGroupCode('ab')).toBe(true)
    expect(isValidGroupCode('default')).toBe(true)
    expect(isValidGroupCode('vip_1')).toBe(true)
    expect(isValidGroupCode('a'.repeat(32))).toBe(true)
    expect(GROUP_CODE_RE.test('premium-x')).toBe(true)
  })

  it('rejects invalid codes', () => {
    expect(isValidGroupCode('a')).toBe(false)
    expect(isValidGroupCode('A')).toBe(false)
    expect(isValidGroupCode('Default')).toBe(false)
    expect(isValidGroupCode('has space')).toBe(false)
    expect(isValidGroupCode('')).toBe(false)
    expect(isValidGroupCode('a'.repeat(33))).toBe(false)
    expect(isValidGroupCode('中文')).toBe(false)
  })
})

describe('buildGroupPut', () => {
  it('never includes code in the body', () => {
    const result = buildGroupPut(
      'default',
      { display_name: '默认', sort: '10' },
      base
    )
    expect(result).toHaveProperty('body')
    if ('body' in result) {
      expect(result.body).toEqual({ display_name: '默认' })
      expect(result.body).not.toHaveProperty('code')
    }
  })

  it('rejects empty display_name', () => {
    expect(
      buildGroupPut('default', { display_name: '   ', sort: '10' }, base)
    ).toEqual({ error: 'display_name_required' })
  })

  it('rejects invalid sort', () => {
    expect(
      buildGroupPut('default', { display_name: 'Default', sort: '1.5' }, base)
    ).toEqual({ error: 'invalid_sort' })
  })

  it('returns no_changes when draft matches initial', () => {
    expect(
      buildGroupPut('default', { display_name: 'Default', sort: '10' }, base)
    ).toEqual({ error: 'no_changes' })
  })

  it('includes sort when only sort changes', () => {
    const result = buildGroupPut(
      'default',
      { display_name: 'Default', sort: '20' },
      base
    )
    expect(result).toEqual({ body: { sort: 20 } })
  })

  it('includes both when both change', () => {
    const result = buildGroupPut(
      'default',
      { display_name: '新名', sort: '0' },
      base
    )
    expect(result).toEqual({
      body: { display_name: '新名', sort: 0 },
    })
  })
})

describe('validateCreateInput', () => {
  it('rejects invalid code', () => {
    expect(
      validateCreateInput({ code: 'X', display_name: '', sort: '0' })
    ).toEqual({ ok: false, error: 'invalid_code' })
  })

  it('allows empty display_name (server falls back to code)', () => {
    expect(
      validateCreateInput({ code: 'vip', display_name: '  ', sort: '5' })
    ).toEqual({
      ok: true,
      body: { code: 'vip', sort: 5 },
    })
  })

  it('passes display_name when set', () => {
    expect(
      validateCreateInput({
        code: 'vip',
        display_name: ' VIP ',
        sort: '1',
      })
    ).toEqual({
      ok: true,
      body: { code: 'vip', display_name: 'VIP', sort: 1 },
    })
  })
})

describe('viewToDraft / groupApiErrorToastKey', () => {
  it('maps view to draft strings', () => {
    expect(viewToDraft(base)).toEqual({
      display_name: 'Default',
      sort: '10',
    })
  })

  it('maps known API codes', () => {
    expect(groupApiErrorToastKey('invalid_code')).toContain('Invalid group')
    expect(groupApiErrorToastKey('exists')).toContain('already exists')
    expect(groupApiErrorToastKey('code_immutable')).toContain(
      'cannot be changed'
    )
    expect(groupApiErrorToastKey('display_name_required')).toContain('required')
    expect(groupApiErrorToastKey('forbidden')).toContain('Root permission')
    expect(groupApiErrorToastKey('root_required')).toContain('Root permission')
    expect(groupApiErrorToastKey('unknown')).toBeNull()
  })
})
