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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import type { PaymentOrphanEvent } from '../api'
import { PaymentOrphanPanel } from '../components/payment-orphan-panel'

const api = vi.hoisted(() => ({
  getPaymentOrphans: vi.fn(),
  creditPaymentOrphan: vi.fn(),
  resolvePaymentOrphan: vi.fn(),
}))
vi.mock('../api', () => api)

const event: PaymentOrphanEvent = {
  id: 11,
  provider: 'waffo_pancake',
  event_type: 'reversal',
  reference_id: 'PCK001',
  status: 'pending_review',
  reason: 'External reversal',
  error: '',
  create_time: 1,
  resolution_note: '',
  can_credit: false,
}

function show(row = event, canCredit = true) {
  api.getPaymentOrphans.mockResolvedValue({
    items: [row],
    total: 1,
    page: 1,
    page_size: 20,
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <PaymentOrphanPanel canCredit={canCredit} />
    </QueryClientProvider>
  )
  return client
}

async function openReview() {
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Review' }))
  return screen.findByRole('dialog', { name: 'Review payment event' })
}

async function fillEvidence(dialog: HTMLElement) {
  const user = userEvent.setup()
  await user.type(
    within(dialog).getByLabelText(
      'Proof (transaction hash or platform order number)'
    ),
    'provider-proof-001'
  )
  await user.type(
    within(dialog).getByLabelText('Note'),
    'Checked external reversal'
  )
}

beforeEach(() => api.resolvePaymentOrphan.mockResolvedValue(undefined))
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

it('opens a labeled review form and never resolves from an immediate empty-note action', async () => {
  show()
  const dialog = await openReview()
  expect(api.resolvePaymentOrphan).not.toHaveBeenCalled()
  expect(within(dialog).getByLabelText('Order number')).toHaveValue('PCK001')
  expect(within(dialog).getByLabelText('Order number')).toHaveAttribute(
    'readonly'
  )
  expect(
    within(dialog).getByRole('button', { name: 'Save review' })
  ).toBeDisabled()
  expect(
    within(dialog).getByText(/Use Refund on the original order/)
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Refunded offline' })
  ).not.toBeInTheDocument()
})

it('saves a partial reversal as pending review with exact decimal facts and evidence', async () => {
  show()
  const dialog = await openReview()
  await fillEvidence(dialog)
  const user = userEvent.setup()
  const amount = within(dialog).getByLabelText(
    'Cumulative external reversal amount'
  )
  await user.clear(amount)
  await user.type(amount, '12.3400')
  await user.type(
    within(dialog).getByLabelText('External reversal currency'),
    'cny'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save review' }))
  await waitFor(() =>
    expect(api.resolvePaymentOrphan).toHaveBeenCalledWith(11, {
      status: 'pending_review',
      trade_no: 'PCK001',
      external_amount: '12.3400',
      external_currency: 'CNY',
      proof: 'provider-proof-001',
      note: 'Checked external reversal',
    })
  )
})

it.each(['refunded', 'never_credited'])(
  'requires a positive external amount before %s',
  async (status) => {
    show()
    const dialog = await openReview()
    const user = userEvent.setup()
    await fillEvidence(dialog)
    await user.selectOptions(
      within(dialog).getByLabelText('Review outcome'),
      status
    )
    await user.type(
      within(dialog).getByLabelText('External reversal currency'),
      'CNY'
    )
    const save = within(dialog).getByRole('button', { name: 'Save review' })
    expect(save).toBeDisabled()
    const amount = within(dialog).getByLabelText(
      'Cumulative external reversal amount'
    )
    await user.clear(amount)
    await user.type(amount, '0.01')
    expect(save).toBeEnabled()
    await user.click(save)
    await waitFor(() =>
      expect(api.resolvePaymentOrphan).toHaveBeenCalledWith(
        11,
        expect.objectContaining({ status, external_amount: '0.01' })
      )
    )
  }
)

it('requires a dismissal reason, explains the removed block, and omits reversal facts', async () => {
  show()
  const dialog = await openReview()
  const user = userEvent.setup()
  await fillEvidence(dialog)
  await user.selectOptions(
    within(dialog).getByLabelText('Review outcome'),
    'dismissed'
  )
  expect(
    within(dialog).getByText(
      'Dismissing this event removes the block on late payment credit.'
    )
  ).toBeVisible()
  const save = within(dialog).getByRole('button', { name: 'Save review' })
  expect(save).toBeDisabled()
  await user.selectOptions(
    within(dialog).getByLabelText('Dismissal reason'),
    'not_this_order'
  )
  await user.click(save)
  await waitFor(() =>
    expect(api.resolvePaymentOrphan).toHaveBeenCalledWith(11, {
      status: 'dismissed',
      trade_no: 'PCK001',
      external_amount: '0',
      external_currency: '',
      proof: 'provider-proof-001',
      note: 'Checked external reversal',
      dismissal_reason: 'not_this_order',
    })
  )
})

it('retains retryable evidence after a failed review and guards a dirty close', async () => {
  api.resolvePaymentOrphan.mockRejectedValueOnce(new Error('Review conflict'))
  show()
  const dialog = await openReview()
  await fillEvidence(dialog)
  const user = userEvent.setup()
  await user.type(
    within(dialog).getByLabelText('External reversal currency'),
    'CNY'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save review' }))
  await waitFor(() => expect(api.resolvePaymentOrphan).toHaveBeenCalledOnce())
  expect(within(dialog).getByLabelText('Note')).toHaveValue(
    'Checked external reversal'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await user.click(await screen.findByRole('button', { name: 'Stay' }))
  expect(within(dialog).getByLabelText('Note')).toHaveValue(
    'Checked external reversal'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save review' }))
  await waitFor(() => expect(api.resolvePaymentOrphan).toHaveBeenCalledTimes(2))
  await waitFor(() => expect(dialog).not.toBeInTheDocument())
})

it('shows linked wallet recovery and cash exposure from the immutable review snapshot', async () => {
  show({
    ...event,
    review: {
      id: 21,
      status: 'pending_review',
      note: 'Partial refund checked',
      proof: 'proof-21',
      trade_no: 'PCK001',
      external_amount: '200',
      external_currency: 'CNY',
      refund_ledger_id: 9,
      deducted_quota: 5000000,
      refund_quota: 4500000,
      refund_amount: '65.70',
      paid_amount: '200',
      paid_currency: 'CNY',
      exposure_amount: '134.30',
      created_at: 2,
      operator_id: 4,
    },
  })
  const dialog = await openReview()
  const snapshot = within(dialog).getByRole('region', {
    name: 'Latest recorded review',
  })
  expect(within(snapshot).getByText('200 CNY')).toBeVisible()
  expect(within(snapshot).getByText('65.70 CNY')).toBeVisible()
  expect(within(snapshot).getByText('134.30 CNY')).toBeVisible()
  expect(within(snapshot).getByText('Wallet credit taken back')).toBeVisible()
  expect(within(snapshot).getByText('Principal cash coverage')).toBeVisible()
})

it('does not present default zero values without a linked refund as settled exposure', async () => {
  show({
    ...event,
    review: {
      id: 21,
      status: 'pending_review',
      note: 'Awaiting reconciliation',
      proof: 'proof-21',
      trade_no: 'PCK001',
      external_amount: '200',
      external_currency: 'CNY',
      refund_ledger_id: 0,
      deducted_quota: 0,
      refund_quota: 0,
      refund_amount: '0',
      paid_amount: '200',
      paid_currency: 'CNY',
      exposure_amount: '0',
      created_at: 2,
      operator_id: 4,
    },
  })
  const dialog = await openReview()
  const snapshot = within(dialog).getByRole('region', {
    name: 'Latest recorded review',
  })
  expect(
    within(snapshot).getByText(
      'No linked refund; cash coverage and exposure have not been calculated.'
    )
  ).toBeVisible()
  expect(within(snapshot).queryByText('0 CNY')).not.toBeInTheDocument()
})

it('keeps review mutations unavailable without finance permission', async () => {
  show(event, false)
  await screen.findByText('PCK001')
  expect(
    screen.queryByRole('button', { name: 'Review' })
  ).not.toBeInTheDocument()
  expect(api.resolvePaymentOrphan).not.toHaveBeenCalled()
})

it('waits for an original order association instead of allowing arbitrary rebinding', async () => {
  show({ ...event, reference_id: '' })
  const dialog = await openReview()
  await fillEvidence(dialog)
  await userEvent
    .setup()
    .type(within(dialog).getByLabelText('External reversal currency'), 'CNY')
  expect(
    within(dialog).getByText(
      'Wait for this payment event to be linked to its original order before reviewing it.'
    )
  ).toBeVisible()
  expect(
    within(dialog).getByRole('button', { name: 'Save review' })
  ).toBeDisabled()
})

it('shows terminal review reasons and operator attribution without mutation controls', async () => {
  show(
    {
      ...event,
      status: 'dismissed',
      review: {
        id: 22,
        status: 'dismissed',
        note: 'Provider canceled reversal',
        proof: 'proof-22',
        dismissal_reason: 'reversal_canceled',
        trade_no: 'PCK001',
        external_amount: '0',
        external_currency: '',
        refund_ledger_id: 0,
        deducted_quota: 0,
        refund_quota: 0,
        refund_amount: '0',
        paid_amount: '200',
        paid_currency: 'CNY',
        exposure_amount: '0',
        created_at: 2,
        operator_id: 81,
      },
    },
    false
  )
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'View review' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Review payment event',
  })
  expect(
    within(dialog).getByText('The provider canceled this reversal')
  ).toBeVisible()
  expect(within(dialog).getByText('Operator ID')).toBeVisible()
  expect(within(dialog).getByText('81')).toBeVisible()
  expect(
    within(dialog).queryByRole('button', { name: 'Save review' })
  ).not.toBeInTheDocument()
})
it.each(['-1', '1e3', '0.000000001', '1000000000000'])(
  'rejects unsupported external amount %s before sending',
  async (value) => {
    show()
    const dialog = await openReview()
    await fillEvidence(dialog)
    const user = userEvent.setup()
    await user.type(
      within(dialog).getByLabelText('External reversal currency'),
      'CNY'
    )
    const amount = within(dialog).getByLabelText(
      'Cumulative external reversal amount'
    )
    await user.clear(amount)
    await user.type(amount, value)
    expect(amount).toHaveAttribute('aria-invalid', 'true')
    expect(
      within(dialog).getByRole('button', { name: 'Save review' })
    ).toBeDisabled()
    expect(api.resolvePaymentOrphan).not.toHaveBeenCalled()
  }
)
