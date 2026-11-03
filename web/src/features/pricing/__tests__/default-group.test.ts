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
import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { useFilters } from '../hooks/use-filters'
import type { PricingModel } from '../types'

const search = vi.hoisted(() => ({ value: {} as Record<string, unknown> }))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useSearch: () => search.value,
}))

function pricingModel(name: string, groups: string[]): PricingModel {
  return {
    id: name.length,
    model_name: name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: groups,
  }
}

const models = [
  pricingModel('default-model', ['default']),
  pricingModel('vip-model', ['vip']),
]

afterEach(() => {
  search.value = {}
  useAuthStore.setState(useAuthStore.getInitialState(), true)
})

describe('default pricing group', () => {
  it('selects default for a visitor who is not signed in', () => {
    const { result } = renderHook(() => useFilters(models, ['default', 'vip']))
    expect(result.current.groupFilter).toBe('default')
    expect(result.current.filteredModels.map((m) => m.model_name)).toEqual([
      'default-model',
    ])
    expect(result.current.hasActiveFilters).toBe(false)
  })

  it('selects the signed-in user group', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'u',
      role: 1,
      group: 'vip',
    })
    const { result } = renderHook(() => useFilters(models, ['default', 'vip']))
    expect(result.current.groupFilter).toBe('vip')
  })

  it('falls back to all groups when the default group is not selectable', () => {
    const { result } = renderHook(() => useFilters(models, ['vip']))
    expect(result.current.groupFilter).toBe('all')
    expect(result.current.filteredModels).toHaveLength(2)
  })

  it('keeps all groups after the viewer switches back to all', () => {
    const { result } = renderHook(() => useFilters(models, ['default', 'vip']))
    act(() => result.current.setGroupFilter('all'))
    expect(result.current.groupFilter).toBe('all')
    expect(result.current.filteredModels).toHaveLength(2)
    act(() => result.current.clearFilters())
    expect(result.current.groupFilter).toBe('default')
  })

  it('uses the group from the URL over the default', () => {
    search.value = { group: 'all' }
    const { result } = renderHook(() => useFilters(models, ['default', 'vip']))
    expect(result.current.groupFilter).toBe('all')
  })
})
