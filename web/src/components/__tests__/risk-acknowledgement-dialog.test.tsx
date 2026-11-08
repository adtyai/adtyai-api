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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { RiskAcknowledgementDialog } from '../risk-acknowledgement-dialog'

type RiskProps = ComponentProps<typeof RiskAcknowledgementDialog>
const firstCheck = { id: 'risk:first', text: 'First risk' }
const secondCheck = { id: 'risk:second', text: 'Second risk' }
const newCheck = { id: 'risk:new', text: 'New risk' }
const changedFirstCheck = { id: 'risk:first', text: 'Revised first risk' }
const duplicateFirst = { id: 'risk:duplicate:first', text: 'Repeated risk' }
const duplicateSecond = { id: 'risk:duplicate:second', text: 'Repeated risk' }
const firstItem = { id: 'information:first', text: 'Repeated information' }
const secondItem = { id: 'information:second', text: 'Repeated information' }
const firstPart = {
  id: 'text:first',
  type: 'input' as const,
  text: 'FIRST',
  placeholder: 'First acknowledgement',
}
const secondPart = {
  id: 'text:second',
  type: 'input' as const,
  text: 'SECOND',
  placeholder: 'Second acknowledgement',
}
const newPart = {
  id: 'text:new',
  type: 'input' as const,
  text: 'NEW',
  placeholder: 'New acknowledgement',
}

async function renderRisk(props: Partial<RiskProps> = {}) {
  const user = userEvent.setup()
  const onConfirm = vi.fn()
  let currentProps: RiskProps = {
    open: true,
    onOpenChange: vi.fn(),
    title: 'Risk acknowledgement',
    description: 'Review the risks before confirming.',
    onConfirm,
    ...props,
  }
  const view = render(<RiskAcknowledgementDialog {...currentProps} />)
  await screen.findByRole('alertdialog', { name: 'Risk acknowledgement' })
  return {
    user,
    onConfirm,
    update: (next: Partial<RiskProps>) => {
      currentProps = { ...currentProps, ...next }
      view.rerender(<RiskAcknowledgementDialog {...currentProps} />)
    },
  }
}

describe('risk checklist identity', () => {
  it('allows confirmation only after every current check and required text match', async () => {
    const { user, onConfirm } = await renderRisk({
      checklist: [firstCheck, secondCheck],
      requiredText: 'CONFIRM',
    })
    const confirm = screen.getByRole('button', { name: 'Confirm' })
    await user.click(confirm)
    expect(onConfirm).not.toHaveBeenCalled()
    await user.click(screen.getByRole('checkbox', { name: 'First risk' }))
    await user.type(screen.getByRole('textbox'), ' CONFIRM ')
    expect(confirm).toBeDisabled()
    await user.click(screen.getByRole('checkbox', { name: 'Second risk' }))
    expect(confirm).toBeEnabled()
    await user.click(confirm)
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('moves a checked risk with its identity when checklist entries reorder', async () => {
    const { user, update } = await renderRisk({
      checklist: [firstCheck, secondCheck],
    })
    const first = screen.getByRole('checkbox', { name: 'First risk' })
    await user.click(first)
    first.focus()

    update({ checklist: [secondCheck, firstCheck] })

    expect(screen.getByRole('checkbox', { name: 'First risk' })).toBe(first)
    expect(first).toBeChecked()
    expect(first).toHaveFocus()
    expect(
      screen.getByRole('checkbox', { name: 'Second risk' })
    ).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('leaves a prepended risk unconfirmed while preserving existing checks', async () => {
    const { user, update } = await renderRisk({ checklist: [firstCheck] })
    await user.click(screen.getByRole('checkbox', { name: 'First risk' }))

    update({ checklist: [newCheck, firstCheck] })
    await user.click(screen.getByRole('checkbox', { name: 'New risk' }))

    expect(screen.getByRole('checkbox', { name: 'First risk' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: 'New risk' })).toBeChecked()
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  })

  it('preserves a checked risk when a different unchecked risk is deleted', async () => {
    const { user, update } = await renderRisk({
      checklist: [firstCheck, secondCheck],
    })
    await user.click(screen.getByRole('checkbox', { name: 'Second risk' }))

    update({ checklist: [secondCheck] })

    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: 'Second risk' })
      ).toBeChecked()
    )
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  })

  it('invalidates an existing check when the same risk ID has new text', async () => {
    const { user, update, onConfirm } = await renderRisk({
      checklist: [firstCheck],
    })
    await user.click(screen.getByRole('checkbox', { name: 'First risk' }))

    update({ checklist: [changedFirstCheck] })

    expect(
      screen.getByRole('checkbox', { name: 'Revised first risk' })
    ).not.toBeChecked()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(onConfirm).not.toHaveBeenCalled()
    update({ checklist: [firstCheck] })
    expect(
      screen.getByRole('checkbox', { name: 'First risk' })
    ).not.toBeChecked()
  })

  it('keeps identical checklist text independent when the two IDs reorder', async () => {
    const { user, update } = await renderRisk({
      checklist: [duplicateFirst, duplicateSecond],
    })
    const original = screen.getAllByRole('checkbox', { name: 'Repeated risk' })
    await user.click(original[0])

    update({ checklist: [duplicateSecond, duplicateFirst] })

    const reordered = screen.getAllByRole('checkbox', { name: 'Repeated risk' })
    expect(reordered[0]).toBe(original[1])
    expect(reordered[0]).not.toBeChecked()
    expect(reordered[1]).toBe(original[0])
    expect(reordered[1]).toBeChecked()
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('requires a fresh check when a removed risk ID is added again', async () => {
    const { user, update } = await renderRisk({
      checklist: [firstCheck, secondCheck],
    })
    await user.click(screen.getByRole('checkbox', { name: 'First risk' }))
    update({ checklist: [secondCheck] })
    update({ checklist: [firstCheck, secondCheck] })

    expect(
      screen.getByRole('checkbox', { name: 'First risk' })
    ).not.toBeChecked()
  })
})

describe('risk confirmation text identity', () => {
  it('preserves input value and focus when required text parts reorder', async () => {
    const { user, update } = await renderRisk({
      requiredTextParts: [firstPart, secondPart],
    })
    const first = screen.getByPlaceholderText('First acknowledgement')
    await user.type(first, 'FIRST')

    update({ requiredTextParts: [secondPart, firstPart] })

    expect(screen.getByPlaceholderText('First acknowledgement')).toBe(first)
    expect(first).toHaveValue('FIRST')
    expect(first).toHaveFocus()
    expect(screen.getByPlaceholderText('Second acknowledgement')).toHaveValue(
      ''
    )
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('requires a prepended input without clearing an existing typed acknowledgement', async () => {
    const { user, update } = await renderRisk({
      requiredTextParts: [firstPart],
    })
    await user.type(
      screen.getByPlaceholderText('First acknowledgement'),
      'FIRST'
    )

    update({ requiredTextParts: [newPart, firstPart] })
    await user.type(screen.getByPlaceholderText('New acknowledgement'), 'NEW')

    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue(
      'FIRST'
    )
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  })

  it('preserves a typed acknowledgement when another unfilled input is deleted', async () => {
    const { user, update } = await renderRisk({
      requiredTextParts: [firstPart, secondPart],
    })
    await user.type(
      screen.getByPlaceholderText('Second acknowledgement'),
      'SECOND'
    )

    update({ requiredTextParts: [secondPart] })

    await waitFor(() =>
      expect(screen.getByPlaceholderText('Second acknowledgement')).toHaveValue(
        'SECOND'
      )
    )
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  })

  it('does not transfer typed text between different IDs with the same required text', async () => {
    const repeatedSecond = { ...secondPart, text: 'FIRST' }
    const { user, update } = await renderRisk({
      requiredTextParts: [firstPart, repeatedSecond],
    })
    await user.type(
      screen.getByPlaceholderText('First acknowledgement'),
      'FIRST'
    )

    update({ requiredTextParts: [repeatedSecond, firstPart] })

    expect(screen.getByPlaceholderText('Second acknowledgement')).toHaveValue(
      ''
    )
    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue(
      'FIRST'
    )
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('invalidates typed text when the same ID changes its required content', async () => {
    const { user, update, onConfirm } = await renderRisk({
      requiredTextParts: [firstPart],
    })
    await user.type(
      screen.getByPlaceholderText('First acknowledgement'),
      'FIRST'
    )

    update({ requiredTextParts: [{ ...firstPart, text: 'REVISED' }] })

    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue('')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(onConfirm).not.toHaveBeenCalled()
    update({ requiredTextParts: [firstPart] })
    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('requires fresh typing for a new ID even when its required text is identical', async () => {
    const { user, update, onConfirm } = await renderRisk({
      requiredTextParts: [firstPart],
    })
    await user.type(
      screen.getByPlaceholderText('First acknowledgement'),
      'FIRST'
    )

    update({ requiredTextParts: [{ ...firstPart, id: 'text:replacement' }] })

    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue('')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('keeps the input focused when a static text part is inserted before it', async () => {
    const { user, update } = await renderRisk({
      requiredTextParts: [firstPart],
    })
    const first = screen.getByPlaceholderText('First acknowledgement')
    await user.type(first, 'FIRST')

    update({
      requiredTextParts: [
        { id: 'text:prefix', type: 'static', text: 'Acknowledgement: ' },
        firstPart,
      ],
    })

    expect(screen.getByPlaceholderText('First acknowledgement')).toBe(first)
    expect(first).toHaveValue('FIRST')
    expect(first).toHaveFocus()
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  })

  it('invalidates unsegmented text after the required text changes and returns', async () => {
    const { user, update } = await renderRisk({ requiredText: 'FIRST' })
    await user.type(screen.getByRole('textbox'), 'FIRST')

    update({ requiredText: 'REVISED' })
    update({ requiredText: 'FIRST' })

    expect(screen.getByRole('textbox')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('requires fresh unsegmented text after segmented confirmation replaces it', async () => {
    const { user, update } = await renderRisk({ requiredText: 'FIRST' })
    await user.type(screen.getByRole('textbox'), 'FIRST')

    update({ requiredTextParts: [firstPart] })
    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue('')
    update({ requiredTextParts: [] })

    expect(screen.getByRole('textbox')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('invalidates a typed part when its ID becomes static and later input again', async () => {
    const { user, update } = await renderRisk({
      requiredTextParts: [firstPart],
    })
    await user.type(
      screen.getByPlaceholderText('First acknowledgement'),
      'FIRST'
    )

    update({ requiredTextParts: [{ ...firstPart, type: 'static' }] })
    update({ requiredTextParts: [firstPart] })

    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it('resets checks and entered text when the dialog closes and reopens', async () => {
    const { user, update } = await renderRisk({
      checklist: [firstCheck],
      requiredTextParts: [firstPart],
    })
    await user.click(screen.getByRole('checkbox', { name: 'First risk' }))
    await user.type(
      screen.getByPlaceholderText('First acknowledgement'),
      'FIRST'
    )
    update({ open: false })
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )

    update({ open: true })

    await screen.findByRole('alertdialog')
    expect(
      screen.getByRole('checkbox', { name: 'First risk' })
    ).not.toBeChecked()
    expect(screen.getByPlaceholderText('First acknowledgement')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  })

  it.each([
    { text: '😀'.repeat(42), rows: '1' },
    { text: '😀'.repeat(43), rows: '2' },
    { text: 'A\nB', rows: '1' },
  ])(
    'preserves code point row counting for $rows rows',
    async ({ text, rows }) => {
      await renderRisk({ requiredText: text })

      expect(screen.getByRole('textbox')).toHaveAttribute('rows', rows)
    }
  )
})

describe('risk information identity', () => {
  it('preserves separate information rows when duplicate text IDs reorder', async () => {
    const { update } = await renderRisk({ items: [firstItem, secondItem] })
    const original = screen.getAllByRole('listitem')

    update({ items: [secondItem, firstItem] })

    const reordered = screen.getAllByRole('listitem')
    expect(reordered).toHaveLength(2)
    expect(reordered[0]).toBe(original[1])
    expect(reordered[1]).toBe(original[0])
  })
})
