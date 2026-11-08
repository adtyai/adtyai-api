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
import type { ColumnFiltersState } from '@tanstack/react-table'
import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { useTableUrlState, type NavigateFn } from '../use-table-url-state'

afterEach(() => localStorage.clear())

it.each<{ condition: string; filters: ColumnFiltersState }>([
  { condition: 'missing', filters: [] },
  { condition: 'empty', filters: [{ id: 'status', value: [] }] },
  { condition: 'a string', filters: [{ id: 'status', value: 'enabled' }] },
  { condition: 'an object', filters: [{ id: 'status', value: {} }] },
])(
  'clears an array URL filter when its next value is $condition',
  ({ filters }) => {
    const search = { page: 4, status: ['enabled'], keyword: 'keep' }
    const navigate = vi.fn((options: Parameters<NavigateFn>[0]) => {
      if (typeof options.search === 'function') return options.search(search)
      return options.search
    })
    const serialize = vi.fn((value: unknown) => value)
    const { result } = renderHook(() =>
      useTableUrlState({
        search,
        navigate,
        columnFilters: [
          { columnId: 'status', searchKey: 'status', type: 'array', serialize },
        ],
      })
    )

    act(() => result.current.onColumnFiltersChange(filters))

    expect(navigate).toHaveBeenCalledTimes(1)
    expect(navigate.mock.results[0]?.value).toStrictEqual({
      page: undefined,
      status: undefined,
      keyword: 'keep',
    })
    expect(serialize).not.toHaveBeenCalled()
  }
)

it('serializes an array without dropping falsey values or changing their order', () => {
  const search = { page: 4, pageSize: 50, status: ['old'], keyword: 'keep' }
  const navigate = vi.fn((options: Parameters<NavigateFn>[0]) => {
    if (typeof options.search === 'function') return options.search(search)
    return options.search
  })
  const serialize = vi.fn((value: unknown) => value)
  const { result } = renderHook(() =>
    useTableUrlState({
      search,
      navigate,
      columnFilters: [
        { columnId: 'status', searchKey: 'status', type: 'array', serialize },
      ],
    })
  )
  const values = [0, '', false, 'enabled']

  act(() =>
    result.current.onColumnFiltersChange(() => [
      { id: 'status', value: values },
    ])
  )

  expect(serialize).toHaveBeenCalledExactlyOnceWith(values)
  expect(navigate.mock.results[0]?.value).toStrictEqual({
    page: undefined,
    pageSize: 50,
    status: values,
    keyword: 'keep',
  })
})
