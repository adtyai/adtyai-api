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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { TagInput } from '../tag-input'

it('removes the last tag when Backspace is pressed in an empty input', async () => {
  const onChange = vi.fn()
  const user = userEvent.setup()
  render(<TagInput value={['first', 'last']} onChange={onChange} />)

  await user.click(screen.getByRole('textbox'))
  await user.keyboard('{Backspace}')

  expect(onChange).toHaveBeenCalledExactlyOnceWith(['first'])
})

it('keeps existing tags when Backspace edits nonempty input text', async () => {
  const onChange = vi.fn()
  const user = userEvent.setup()
  render(<TagInput value={['first']} onChange={onChange} />)

  await user.type(screen.getByRole('textbox'), 'draft{Backspace}')

  expect(screen.getByRole('textbox')).toHaveValue('draf')
  expect(onChange).not.toHaveBeenCalled()
})

it('removes every matching tag when the last tag value is repeated', () => {
  const onChange = vi.fn()
  render(<TagInput value={['repeat', 'keep', 'repeat']} onChange={onChange} />)

  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Backspace' })

  expect(onChange).toHaveBeenCalledExactlyOnceWith(['keep'])
})

it('removes an externally supplied empty last tag on Backspace', () => {
  const onChange = vi.fn()
  render(<TagInput value={['keep', '']} onChange={onChange} />)

  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Backspace' })

  expect(onChange).toHaveBeenCalledExactlyOnceWith(['keep'])
})

it('emits the remaining tags when the supplied array has a sparse last entry', () => {
  const onChange = vi.fn()
  const tags = ['keep']
  tags.length = 2
  render(<TagInput value={tags} onChange={onChange} />)

  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Backspace' })

  expect(onChange).toHaveBeenCalledExactlyOnceWith(['keep'])
})

it('does not emit a change when Backspace is pressed with no tags', () => {
  const onChange = vi.fn()
  render(<TagInput value={[]} onChange={onChange} />)

  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Backspace' })

  expect(onChange).not.toHaveBeenCalled()
})
