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

import { GroupHealthSection } from '../group-health-section'

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <GroupHealthSection />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { success_min: '0.95', stale_seconds: 21600, heartbeat_at: 0 },
    },
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it('shows the threshold as a percentage and the stale window in minutes, and saves them back', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: { saved: true } } })
  show()
  const percent = await screen.findByLabelText('Healthy at or above (%)')
  const minutes = screen.getByLabelText(
    'Monitor interrupted after (minutes without samples)'
  )
  await waitFor(() => expect(percent).toHaveValue('95'))
  expect(minutes).toHaveValue('360')
  const user = userEvent.setup()
  await user.clear(percent)
  await user.type(percent, '90.5')
  await user.clear(minutes)
  await user.type(minutes, '30')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/admin/group-health', {
      success_min: '0.905',
      stale_seconds: 1800,
    })
  )
})

it('rejects a percentage above 100', async () => {
  show()
  const percent = await screen.findByLabelText('Healthy at or above (%)')
  await waitFor(() => expect(percent).toHaveValue('95'))
  const user = userEvent.setup()
  await user.clear(percent)
  await user.type(percent, '101')
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
})

it('tells other admins that only Root can change it', () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  show()
  expect(
    screen.getByText('Only Root can change group health settings.')
  ).toBeInTheDocument()
})
