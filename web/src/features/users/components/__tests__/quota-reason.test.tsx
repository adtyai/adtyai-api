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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { UserQuotaDialog } from '../user-quota-dialog'

const api = vi.hoisted(() => ({ adjustUserQuota: vi.fn() }))
vi.mock('../../api', () => api)

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG },
  })
})

afterEach(() => {
  vi.clearAllMocks()
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG },
  })
})

function renderDialog(currentQuota = 0, currentLiveQuota?: number) {
  render(
    <UserQuotaDialog
      open
      onOpenChange={() => {}}
      userId={7}
      currentQuota={currentQuota}
      currentLiveQuota={currentLiveQuota}
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
    await user.type(
      screen.getByLabelText('Reason (admins only)'),
      'set negative'
    )
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Subtract' }))
    await user.clear(screen.getByPlaceholderText(/Enter amount/))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '5')
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    expect(api.adjustUserQuota).not.toHaveBeenCalled()
  })

  it.each([
    { mode: 'Add', preview: 'Current quota: $1 +$0.5 = $1.5' },
    { mode: 'Subtract', preview: 'Current quota: $1 -$0.5 = $0.5' },
  ])('previews $mode against the live balance', async ({ mode, preview }) => {
    const user = userEvent.setup()
    renderDialog(1_000_000, 500_000)
    await user.click(screen.getByRole('button', { name: mode }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '0.5')

    expect(screen.getByText(preview)).toBeVisible()
  })

  it.each([
    { raw: 1_000_000, live: 500_000, blocked: true },
    { raw: 0, live: 2_000_000, blocked: false },
  ])(
    'checks subtraction against live $live rather than raw $raw',
    async ({ raw, live, blocked }) => {
      const user = userEvent.setup()
      renderDialog(raw, live)
      await user.click(screen.getByRole('button', { name: 'Subtract' }))
      await user.type(screen.getByPlaceholderText(/Enter amount/), '2')
      await user.type(
        screen.getByLabelText('Reason (admins only)'),
        'reconcile'
      )

      const confirm = screen.getByRole('button', { name: 'Confirm' })
      if (blocked) {
        expect(confirm).toBeDisabled()
        expect(
          screen.getByText('Adjustment cannot make the balance negative')
        ).toBeVisible()
      } else {
        expect(confirm).toBeEnabled()
        expect(screen.getByText('Current quota: $4 -$2 = $2')).toBeVisible()
      }
    }
  )

  it('blocks a nonnegative raw override when its pending adjustment makes live quota negative', async () => {
    const user = userEvent.setup()
    renderDialog(1_000_000, 500_000)
    await user.click(screen.getByRole('button', { name: 'Override' }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '0.5')
    await user.type(screen.getByLabelText('Reason (admins only)'), 'reconcile')

    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    expect(
      screen.getByText('Adjustment cannot make the balance negative')
    ).toBeVisible()
    expect(api.adjustUserQuota).not.toHaveBeenCalled()
  })

  it('submits the raw override target while previewing a zero live balance', async () => {
    api.adjustUserQuota.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderDialog(1_000_000, 500_000)
    await user.click(screen.getByRole('button', { name: 'Override' }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '1')
    await user.type(screen.getByLabelText('Reason (admins only)'), 'reconcile')

    expect(screen.getByText('Current quota: $1 → $0')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(api.adjustUserQuota).toHaveBeenCalledWith(
        expect.objectContaining({
          mode: 'override',
          value: 500_000,
        })
      )
    )
  })

  it('allows a zero raw override while retaining a positive pending balance in its preview', async () => {
    api.adjustUserQuota.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderDialog(1_000_000, 2_000_000)
    await user.click(screen.getByRole('button', { name: 'Override' }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '0')
    await user.type(screen.getByLabelText('Reason (admins only)'), 'reconcile')

    expect(screen.getByText('Current quota: $4 → $2')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(api.adjustUserQuota).toHaveBeenCalledWith(
        expect.objectContaining({
          mode: 'override',
          value: 0,
        })
      )
    )
  })

  it('rejects a negative raw override even when the live balance would stay positive', async () => {
    const user = userEvent.setup()
    renderDialog(0, 1_000_000)
    await user.click(screen.getByRole('button', { name: 'Override' }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '-1')
    await user.type(screen.getByLabelText('Reason (admins only)'), 'reconcile')

    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled()
    expect(api.adjustUserQuota).not.toHaveBeenCalled()
  })

  it('allows an add to reduce legacy live debt without requiring a full repayment', async () => {
    api.adjustUserQuota.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderDialog(0, -1_000_000)
    await user.type(screen.getByPlaceholderText(/Enter amount/), '0.5')
    await user.type(
      screen.getByLabelText('Reason (admins only)'),
      'partial repayment'
    )

    expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(api.adjustUserQuota).toHaveBeenCalledWith(
        expect.objectContaining({
          mode: 'add',
          value: 250_000,
        })
      )
    )
  })

  it('converts a currency override input to the raw quota target before applying the live offset', async () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
    api.adjustUserQuota.mockResolvedValueOnce({ success: true })
    const user = userEvent.setup()
    renderDialog(1_000_000, 500_000)
    await user.click(screen.getByRole('button', { name: 'Override' }))
    await user.type(screen.getByPlaceholderText(/Enter amount/), '7')
    await user.type(screen.getByLabelText('Reason (admins only)'), 'reconcile')

    expect(screen.getByText('Current quota: ¥7 → ¥0')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(api.adjustUserQuota).toHaveBeenCalledWith(
        expect.objectContaining({
          mode: 'override',
          value: 500_000,
        })
      )
    )
  })
})
