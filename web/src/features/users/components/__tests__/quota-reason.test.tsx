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
import { afterEach, describe, expect, it, vi } from 'vitest'

import { UserQuotaDialog } from '../user-quota-dialog'

const api = vi.hoisted(() => ({ adjustUserQuota: vi.fn() }))
vi.mock('../../api', () => api)

afterEach(() => vi.clearAllMocks())

function renderDialog(currentQuota = 0) {
  render(
    <UserQuotaDialog
      open
      onOpenChange={() => {}}
      userId={7}
      currentQuota={currentQuota}
      onSuccess={() => {}}
    />
  )
}

describe('UserQuotaDialog reason (adty task 7.2)', () => {
  it('keeps Confirm disabled until a reason is entered', async () => {
    const user = userEvent.setup()
    renderDialog()
    await user.type(screen.getByPlaceholderText(/Enter amount/), '5')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    await user.type(screen.getByLabelText('Reason (admins only)'), 'refund')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  })

  it('reuses the idempotency key when a failed request is retried', async () => {
    api.adjustUserQuota
      .mockResolvedValueOnce({ success: false, message: 'network' })
      .mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderDialog()
    await user.type(screen.getByPlaceholderText(/Enter amount/), '5')
    await user.type(screen.getByLabelText('Reason (admins only)'), ' goodwill ')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(api.adjustUserQuota).toHaveBeenCalledTimes(1))
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(api.adjustUserQuota).toHaveBeenCalledTimes(2))
    const [first, second] = api.adjustUserQuota.mock.calls.map((c) => c[0])
    expect(first.reason).toBe('goodwill')
    expect(first.idempotency_key).toBe(second.idempotency_key)
    expect(first.idempotency_key).not.toBe('')
  })

  it('does not submit an override or subtract that would write a negative balance', async () => {
    const user = userEvent.setup()
    renderDialog(1_000_000)
    await user.click(screen.getByRole('button', { name: 'Override' }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '-1')
    await user.type(screen.getByLabelText('Reason (admins only)'), 'set negative')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Subtract' }))
    await user.clear(screen.getByPlaceholderText(/Enter amount/))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '5')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    expect(api.adjustUserQuota).not.toHaveBeenCalled()
  })
})
