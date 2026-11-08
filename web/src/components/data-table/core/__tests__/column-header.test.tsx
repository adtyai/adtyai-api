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
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'

import { DataTableColumnHeader } from '../column-header'

const rows = [{ name: 'Beta' }, { name: 'Alpha' }]

function Fixture(props: { sortable?: boolean }) {
  const table = useReactTable({
    data: rows,
    columns: [{ accessorKey: 'name', enableSorting: props.sortable }],
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })
  return (
    <>
      {table.getVisibleLeafColumns().map((column) => (
        <DataTableColumnHeader key={column.id} column={column} title='Name' />
      ))}
      <ol aria-label='Rows'>
        {table.getRowModel().rows.map((row) => (
          <li key={row.id}>{row.original.name}</li>
        ))}
      </ol>
    </>
  )
}

it('updates the sort direction glyph and row order through Asc and Desc menu actions', async () => {
  const user = userEvent.setup()
  render(<Fixture />)
  const trigger = screen.getByRole('button', { name: 'Name' })
  expect(trigger.querySelector('.lucide-chevrons-up-down')).toBeInTheDocument()

  await user.click(trigger)
  await user.click(await screen.findByRole('menuitem', { name: 'Asc' }))

  expect(trigger.querySelector('.lucide-arrow-up')).toBeInTheDocument()
  expect(
    within(screen.getByRole('list', { name: 'Rows' }))
      .getAllByRole('listitem')
      .map((item) => item.textContent)
  ).toEqual(['Alpha', 'Beta'])

  await user.click(trigger)
  await user.click(await screen.findByRole('menuitem', { name: 'Desc' }))

  expect(trigger.querySelector('.lucide-arrow-down')).toBeInTheDocument()
  expect(
    within(screen.getByRole('list', { name: 'Rows' }))
      .getAllByRole('listitem')
      .map((item) => item.textContent)
  ).toEqual(['Beta', 'Alpha'])
})

it('hides a hideable column when its Hide menu action is selected', async () => {
  const user = userEvent.setup()
  render(<Fixture />)

  await user.click(screen.getByRole('button', { name: 'Name' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Hide' }))

  expect(screen.queryByText('Name')).not.toBeInTheDocument()
})

it('renders a nonsortable column title without a menu button', () => {
  render(<Fixture sortable={false} />)

  expect(screen.getByText('Name')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Name' })).not.toBeInTheDocument()
})
