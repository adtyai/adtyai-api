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
import {
  getCoreRowModel,
  useReactTable,
  type ColumnFiltersState,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, it, vi } from 'vitest'

import { DataTableFacetedFilter } from '../faceted-filter'

function FallbackIcon(props: { className?: string }) {
  return (
    <svg className={props.className} role='img' aria-label='Fallback icon' />
  )
}

const options = [
  {
    value: 'a',
    label: 'Alpha',
    iconNode: <svg role='img' aria-label='Provided icon' />,
    icon: FallbackIcon,
    count: 0,
  },
  { value: 'b', label: 'Beta', iconNode: 0, icon: FallbackIcon },
  { value: 'c', label: 'Gamma' },
]
const rows = [{ status: 'a' }, { status: 'b' }, { status: 'c' }]
const facetCounts = new Map([
  ['a', 9],
  ['b', 7],
  ['c', 0],
])

function Fixture(props: {
  singleSelect?: boolean
  onChange?: (value: unknown) => void
}) {
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const table = useReactTable({
    data: rows,
    columns: [{ accessorKey: 'status' }],
    getCoreRowModel: getCoreRowModel(),
    getFacetedUniqueValues: () => () => facetCounts,
    state: { columnFilters },
    onColumnFiltersChange: (updater) => {
      const next =
        typeof updater === 'function' ? updater(columnFilters) : updater
      setColumnFilters(next)
      props.onChange?.(next.find((filter) => filter.id === 'status')?.value)
    },
  })
  return (
    <DataTableFacetedFilter
      column={table.getColumn('status')}
      title='Status'
      options={options}
      singleSelect={props.singleSelect}
    />
  )
}

it('prefers a truthy icon node and falls back to the icon for a zero icon node', async () => {
  const user = userEvent.setup()
  render(<Fixture />)

  await user.click(screen.getByRole('button', { name: 'Status' }))

  const alpha = await screen.findByRole('option', { name: /Alpha/ })
  const beta = screen.getByRole('option', { name: /Beta/ })
  expect(
    within(alpha).getByRole('img', { name: 'Provided icon' })
  ).toBeVisible()
  expect(
    within(alpha).queryByRole('img', { name: 'Fallback icon' })
  ).not.toBeInTheDocument()
  expect(within(beta).getByRole('img', { name: 'Fallback icon' })).toBeVisible()
  expect(
    within(screen.getByRole('option', { name: 'Gamma' })).queryByRole('img')
  ).not.toBeInTheDocument()
})

it('shows an explicit zero count before facet counts and omits a zero facet count', async () => {
  const user = userEvent.setup()
  render(<Fixture />)

  await user.click(screen.getByRole('button', { name: 'Status' }))

  const alpha = await screen.findByRole('option', { name: /Alpha/ })
  expect(within(alpha).getByText('0')).toBeVisible()
  expect(within(alpha).queryByText('9')).not.toBeInTheDocument()
  expect(
    within(screen.getByRole('option', { name: /Beta/ })).getByText('7')
  ).toBeVisible()
  expect(
    within(screen.getByRole('option', { name: 'Gamma' })).queryByText('0')
  ).not.toBeInTheDocument()
})

it('retains multi-selection insertion order and emits no filter after Clear filters', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<Fixture onChange={onChange} />)
  await user.click(screen.getByRole('button', { name: 'Status' }))

  await user.click(await screen.findByRole('option', { name: /Beta/ }))
  await user.click(screen.getByRole('option', { name: /Alpha/ }))
  expect(onChange).toHaveBeenLastCalledWith(['b', 'a'])

  await user.click(screen.getByRole('option', { name: /Beta/ }))
  expect(onChange).toHaveBeenLastCalledWith(['a'])
  await user.click(screen.getByRole('option', { name: /Beta/ }))
  expect(onChange).toHaveBeenLastCalledWith(['a', 'b'])
  await user.click(screen.getByRole('option', { name: 'Clear filters' }))

  expect(onChange).toHaveBeenLastCalledWith(undefined)
  expect(screen.getByRole('button', { name: 'Status' })).toBeVisible()
})

it('replaces a single selection and toggles it off when selected again', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<Fixture singleSelect onChange={onChange} />)
  await user.click(screen.getByRole('button', { name: 'Status' }))

  await user.click(await screen.findByRole('option', { name: /Beta/ }))
  await user.click(screen.getByRole('option', { name: /Alpha/ }))
  expect(onChange).toHaveBeenLastCalledWith(['a'])
  await user.click(screen.getByRole('option', { name: /Alpha/ }))

  expect(onChange).toHaveBeenLastCalledWith(undefined)
})
