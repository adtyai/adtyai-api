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

import type { SiteEarningReview } from '../api'
import { SiteEarningReviewPanel } from '../components/site-earning-review-panel'

const api = vi.hoisted(() => ({
  getSiteEarningReviews: vi.fn(),
  resolveSiteEarningReview: vi.fn(),
}))
vi.mock('../api', () => api)
const verification = vi.hoisted(() => ({ requestVerification: vi.fn() }))
vi.mock(
  '@/features/auth/secure-verification/hooks/use-secure-verification',
  () => ({
    useSecureVerification: () => ({
      requestVerification: verification.requestVerification,
      dialogProps: { state: { phase: 'idle' } },
      isActive: false,
      cancel: vi.fn(),
    }),
  })
)
vi.mock(
  '@/features/auth/secure-verification/components/secure-verification-dialog',
  () => ({ SecureVerificationDialog: () => null })
)

const event: SiteEarningReview = {
  operation_key: 'legacy-adjust:task:41:2',
  root_operation_key: 'legacy:task:41',
  site_id: 7,
  site_owner_user_id: 70,
  user_id: 701,
  quota: 250000,
  markup: 1.2,
  cost_factor: 0.8,
  risk_flag: '',
  review_status: 'needs_review',
  day: '2026-11-01',
  created_at: 1,
}

function show(canResolve = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteEarningReviewPanel canResolve={canResolve} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  api.getSiteEarningReviews.mockResolvedValue({ total: 1, items: [event] })
  api.resolveSiteEarningReview.mockResolvedValue(undefined)
  verification.requestVerification.mockResolvedValue({ proof_token: 'proof' })
})
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

it('lists unresolved earnings and hides the action without write permission', async () => {
  show(false)
  const table = await screen.findByRole('table', {
    name: 'Site earnings needing review',
  })
  expect(table).toHaveTextContent('legacy:task:41')
  expect(api.getSiteEarningReviews).toHaveBeenCalledWith(1, 20)
  expect(
    within(table).queryByRole('button', { name: 'Review' })
  ).not.toBeInTheDocument()
})

it('requires a decision and reason, then verifies with the exact body', async () => {
  show()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Review' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Review site earnings',
  })
  const save = within(dialog).getByRole('button', { name: 'Save review' })
  expect(save).toBeDisabled()
  await user.selectOptions(
    within(dialog).getByLabelText('Original charge'),
    'released'
  )
  expect(save).toBeDisabled()
  await user.type(
    within(dialog).getByLabelText('Reason (required)'),
    '  matched the old invoice  '
  )
  await user.click(save)
  const body = {
    operation_key: 'legacy-adjust:task:41:2',
    original_charge: 'released',
    reason: 'matched the old invoice',
  }
  await waitFor(() =>
    expect(api.resolveSiteEarningReview).toHaveBeenCalledWith(body, 'proof')
  )
  expect(verification.requestVerification).toHaveBeenCalledWith(
    expect.objectContaining({
      scope: 'adty.site_earning.review',
      context: body,
    })
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Review site earnings' })
    ).not.toBeInTheDocument()
  )
})

it('keeps the dialog open when verification is cancelled', async () => {
  verification.requestVerification.mockResolvedValueOnce(null)
  show()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Review' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Review site earnings',
  })
  await user.selectOptions(
    within(dialog).getByLabelText('Original charge'),
    'skipped'
  )
  await user.type(
    within(dialog).getByLabelText('Reason (required)'),
    'no record'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save review' }))
  await waitFor(() =>
    expect(verification.requestVerification).toHaveBeenCalledOnce()
  )
  expect(api.resolveSiteEarningReview).not.toHaveBeenCalled()
  expect(dialog).toBeVisible()
})
