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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { UserLimitsFields } from '../user-limits-fields'

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

afterEach(() => vi.restoreAllMocks())

describe('UserLimitsFields', () => {
  it('shows -1 as empty and saves an emptied field as -1 and 0 as no limit', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { rpm: 60, tpm: -1, concurrency: 2 } },
    })
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, data: { rpm: -1, tpm: 0, concurrency: 2 } },
    })
    const user = userEvent.setup()
    render(<UserLimitsFields userId={7} canEdit />, { wrapper })
    await waitFor(() => expect(screen.getByLabelText('RPM')).toHaveValue(60))
    expect(screen.getByLabelText('TPM')).toHaveValue(null)
    await user.clear(screen.getByLabelText('RPM'))
    await user.type(screen.getByLabelText('TPM'), '0')
    await user.click(screen.getByRole('button', { name: 'Save limits' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/adty/admin/users/7/limits', {
        rpm: -1,
        tpm: 0,
        concurrency: 2,
      })
    )
  })

  it('is read-only without the edit permission', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { rpm: -1, tpm: -1, concurrency: -1 } },
    })
    render(<UserLimitsFields userId={7} canEdit={false} />, { wrapper })
    await waitFor(() => expect(screen.getByLabelText('RPM')).toBeDisabled())
    expect(
      screen.queryByRole('button', { name: 'Save limits' })
    ).not.toBeInTheDocument()
  })
})
