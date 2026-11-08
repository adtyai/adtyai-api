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
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SiteApplyCard } from '../components/site-apply-card'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('keeps application disabled while the agreement fails and enables it only after readable terms and version load', async () => {
  let loaded = false
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/adty/site') {
      return { data: { success: true, data: { site_agreement_version: 4 } } }
    }
    if (!loaded) throw new Error('Synthetic terms failure')
    return {
      data: {
        success: true,
        data: '## Synthetic reseller terms\nRead before applying.',
      },
    }
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SiteApplyCard />
    </QueryClientProvider>
  )
  await userEvent.type(screen.getByLabelText('Subdomain'), 'synthetic-shop')
  await userEvent.click(screen.getByRole('checkbox'))
  const apply = screen.getByRole('button', {
    name: 'Apply for a reseller site',
  })
  expect(apply).toBeDisabled()
  loaded = true
  await userEvent.click(await screen.findByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByRole('heading', { name: 'Synthetic reseller terms' })
  ).toBeInTheDocument()
  expect(apply).toBeEnabled()
  await userEvent.click(apply)
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/site/apply',
      expect.objectContaining({
        subdomain: 'synthetic-shop',
        agree_version: '4',
      })
    )
  )
  client.clear()
})
