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
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { PromoPanel } from '../components/promo-panel'

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={client}>{node}</QueryClientProvider>
}

describe('PromoPanel', () => {
  it('does not offer a new promo code', async () => {
    render(wrap(<PromoPanel canCreate canWrite />))
    expect(
      await screen.findByText('Promo codes are not available')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'New promo code' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  })

  it('stays closed when the viewer cannot edit', async () => {
    render(wrap(<PromoPanel canCreate={false} canWrite={false} />))
    expect(
      await screen.findByText('Promo codes are not available')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'New promo code' })
    ).not.toBeInTheDocument()
  })
})
