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
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { CodeBlock, CodeBlockCopyButton } from '../code-block'

const originalClipboard = Object.getOwnPropertyDescriptor(
  navigator,
  'clipboard'
)

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  cleanup()
  vi.clearAllTimers()
  vi.useRealTimers()
  if (originalClipboard) {
    Object.defineProperty(navigator, 'clipboard', originalClipboard)
  } else {
    Reflect.deleteProperty(navigator, 'clipboard')
  }
})

it('keeps the copied label for the full timeout after the latest copy and releases timers on unmount', async () => {
  let copied = ''
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: async (value: string) => {
        copied = value
      },
    },
  })
  const view = render(
    <CodeBlock code='const answer = 42' language='javascript'>
      <CodeBlockCopyButton />
    </CodeBlock>
  )

  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Copy code' }))
  })
  expect(copied).toBe('const answer = 42')
  act(() => vi.advanceTimersByTime(1000))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Copied!' }))
  })
  act(() => vi.advanceTimersByTime(1000))
  expect(screen.getByRole('button', { name: 'Copied!' })).toBeVisible()

  view.unmount()
  expect(vi.getTimerCount()).toBe(0)
})

it('does not report completion when a pending clipboard write resolves after unmount', async () => {
  let finishCopy!: () => void
  const pending = new Promise<void>((resolve) => {
    finishCopy = resolve
  })
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: () => pending },
  })
  const onCopy = vi.fn()
  const view = render(
    <CodeBlock code='const answer = 42' language='javascript'>
      <CodeBlockCopyButton onCopy={onCopy} />
    </CodeBlock>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Copy code' }))
  view.unmount()

  await act(async () => {
    finishCopy()
    await pending
  })

  expect(onCopy).not.toHaveBeenCalled()
  expect(vi.getTimerCount()).toBe(0)
})
