/*
Copyright (C) 2023-2026 QuantumNous
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { RiskReviewsSettings } from '../index'
import type { RiskReview } from '../types'

const sample: RiskReview = {
  id: 11,
  kind: 'first_topup_bonus',
  user_id: 9,
  order_no: 'ORD-R-1',
  amount_usd: 5,
  status: 'pending',
  reason: 'device_risk',
  payload: '',
  resolved_by: 0,
  created_at: '2026-10-01T00:00:00Z',
  resolved_at: null,
}

const {
  listRiskReviews,
  getRiskReview,
  approveRiskReview,
  rejectRiskReview,
} = vi.hoisted(() => ({
  listRiskReviews: vi.fn(),
  getRiskReview: vi.fn(),
  approveRiskReview: vi.fn(),
  rejectRiskReview: vi.fn(),
}))

vi.mock('../api', () => ({
  listRiskReviews,
  getRiskReview,
  approveRiskReview,
  rejectRiskReview,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RiskReviewsSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  listRiskReviews.mockResolvedValue({ rows: [sample], total: 1 })
  getRiskReview.mockResolvedValue(sample)
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('RiskReviewsSettings ConfirmActionDialog', () => {
  it('mounts dialog when Approve is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('ORD-R-1')
    await user.click(screen.getByRole('button', { name: 'Approve' }))

    expect(
      await screen.findByRole('heading', { name: 'Approve risk review' })
    ).toBeInTheDocument()
  })
})
