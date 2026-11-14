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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PriceSnapshotBackfillSection } from '../price-snapshot-backfill-section'

const report = {
  dry_run: true,
  filled_tasks: 1,
  filled_operations: 1,
  skipped: 2,
  already_complete: 0,
  amounts_preserved: true,
  more_remain: false,
  skipped_sample: [
    {
      kind: 'task',
      id: 'snap-skip-task',
      reason: 'missing_stored_group_ratio',
    },
  ],
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: report },
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { ...report, dry_run: false, filled_tasks: 1, skipped: 2 },
    },
  })
})

it('hides the backfill controls from non-Root administrators', () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <PriceSnapshotBackfillSection />
    </QueryClientProvider>
  )
  expect(
    screen.queryByText('Historical price snapshots')
  ).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalled()
})

it('lets Root preview skipped rows and confirm a non-rewriting backfill', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <PriceSnapshotBackfillSection />
    </QueryClientProvider>
  )
  expect(
    await screen.findByRole('heading', { name: 'Historical price snapshots' })
  ).toBeVisible()
  expect(await screen.findByText(/missing_stored_group_ratio/)).toBeVisible()
  await userEvent.click(
    screen.getByRole('button', { name: 'Backfill stored snapshots' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/adty/admin/billing/price-snapshot-backfill',
      { dry_run: false, limit: 200 }
    )
  )
})
