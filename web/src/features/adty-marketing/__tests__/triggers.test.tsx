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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { MarketingTriggers } from '../api'
import { TriggersPanel } from '../components/triggers-panel'

const api = vi.hoisted(() => ({
  getMarketingTriggers: vi.fn(),
  saveMarketingTriggers: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={client}>{node}</QueryClientProvider>
}

const triggers: MarketingTriggers = {
  unpaid: { enabled: false, days: 3, subjects: {}, bodies: {} },
  low_balance: { enabled: false, usd: '1', subjects: {}, bodies: {} },
  winback: {
    enabled: false,
    days: 30,
    subjects: { en: 'We miss you' },
    bodies: { en: '<p>Come back</p>' },
  },
  batch_size: 200,
}

beforeEach(() => {
  vi.clearAllMocks()
  api.getMarketingTriggers.mockResolvedValue(structuredClone(triggers))
  api.saveMarketingTriggers.mockImplementation(async (body) => body)
})

describe('TriggersPanel', () => {
  it('keeps a rule switch off until its English version is written', async () => {
    render(wrap(<TriggersPanel canWrite />))
    const unpaid = await screen.findByRole('switch', {
      name: 'Used the API but never topped up',
    })
    expect(unpaid).toHaveAttribute('aria-disabled', 'true')
    const winback = screen.getByRole('switch', {
      name: 'Win back paying users',
    })
    expect(winback).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('saves the changed threshold and switch', async () => {
    const user = userEvent.setup()
    render(wrap(<TriggersPanel canWrite />))
    await user.click(
      await screen.findByRole('switch', { name: 'Win back paying users' })
    )
    const days = screen.getByLabelText('Days without calls')
    await user.clear(days)
    await user.type(days, '45')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(api.saveMarketingTriggers).toHaveBeenCalled())
    const body = api.saveMarketingTriggers.mock.calls[0][0] as MarketingTriggers
    expect(body.winback).toMatchObject({ enabled: true, days: 45 })
  })
})
