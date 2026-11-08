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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import type { User } from '../../types'
import { DataTableBulkActions } from '../data-table-bulk-actions'

const users: User[] = [1, 2].map((id) => ({
  id,
  username: `user-${id}`,
  display_name: '',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
}))

function Fixture(props: { selected?: boolean }) {
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>(
    props.selected ? { 0: true, 1: true } : {}
  )
  const table = useReactTable({
    data: users,
    columns: [{ accessorKey: 'username' }],
    getCoreRowModel: getCoreRowModel(),
    state: { rowSelection },
    onRowSelectionChange: setRowSelection,
  })
  return (
    <TooltipProvider>
      <DataTableBulkActions table={table} />
    </TooltipProvider>
  )
}

it('does not show a user bulk toolbar with no selected rows', () => {
  render(<Fixture />)

  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
})

it('shows the selected user count and clears rows when Clear selection is clicked', async () => {
  const user = userEvent.setup()
  render(<Fixture selected />)
  const toolbar = screen.getByRole('toolbar', {
    name: 'Bulk actions for 2 selected records',
  })

  expect(within(toolbar).getByLabelText('2 selected')).toHaveTextContent('2')
  await user.click(
    within(toolbar).getByRole('button', { name: 'Clear selection' })
  )

  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
})

it('clears selected users when Escape is pressed in the bulk toolbar', async () => {
  const user = userEvent.setup()
  render(<Fixture selected />)
  screen
    .getByRole('toolbar', { name: 'Bulk actions for 2 selected records' })
    .focus()

  await user.keyboard('{Escape}')

  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
})
