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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ReferralPublicSettings } from '@/features/referral/types'
import { useAuthStore } from '@/stores/auth-store'

import { AffiliateProgram } from '../index'
import { estimateCommission } from '../lib/estimate'

const api = vi.hoisted(() => ({ getReferralPublicSettings: vi.fn() }))
vi.mock('@/features/referral/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/referral/api')>()),
  ...api,
}))
vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children: ReactNode }) => <div>{props.children}</div>,
}))

const openProgram: ReferralPublicSettings = {
  enabled: true,
  default_rate: 12,
  freeze_days: 7,
  min_withdraw_usd: 20,
  usdt_withdraw_fee: 2,
  cookie_ttl_days: 30,
}

function renderPage(settings: ReferralPublicSettings) {
  api.getReferralPublicSettings.mockResolvedValue(settings)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rootRoute = createRootRoute()
  const page = createRoute({
    getParentRoute: () => rootRoute,
    path: '/affiliate',
    component: AffiliateProgram,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([page]),
    history: createMemoryHistory({ initialEntries: ['/affiliate'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  useAuthStore.getState().auth.reset()
})

describe('AffiliateProgram', () => {
  it('shows the rate and rules from the public settings', async () => {
    renderPage(openProgram)
    expect(
      await screen.findByText(
        'Earn 12% of every top-up made by the people you invite, for as long as they stay.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'Commissions become withdrawable 7 days after the top-up.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'Withdrawals to USDT, Alipay or WeChat Pay start at $20.00.'
      )
    ).toBeVisible()
    expect(
      screen.getByText('USDT withdrawals have a network fee of 2 USDT.')
    ).toBeVisible()
  })

  it('shows not open instead of 0% when no rate is set', async () => {
    renderPage({ ...openProgram, default_rate: null })
    expect(
      await screen.findByText(
        'The affiliate program is not open yet. Check back soon.'
      )
    ).toBeVisible()
    expect(screen.queryByText(/0%/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Sign up to start earning' })
    ).not.toBeInTheDocument()
  })

  it('shows not open when the program is switched off', async () => {
    renderPage({ ...openProgram, enabled: false })
    expect(
      await screen.findByText(
        'The affiliate program is not open yet. Check back soon.'
      )
    ).toBeVisible()
  })

  it('leaves out withdrawal rules that are not set', async () => {
    renderPage({
      ...openProgram,
      min_withdraw_usd: null,
      usdt_withdraw_fee: null,
    })
    await screen.findByText('Rules')
    expect(screen.queryByText(/start at/)).not.toBeInTheDocument()
    expect(screen.queryByText(/network fee/)).not.toBeInTheDocument()
  })

  it('sends a visitor to sign up', async () => {
    renderPage(openProgram)
    expect(
      await screen.findByRole('button', { name: 'Sign up to start earning' })
    ).toHaveAttribute('href', '/sign-up')
  })

  it('sends a signed-in user to the referral page', async () => {
    useAuthStore.getState().auth.setUser({ id: 7, username: 'amy', role: 1 })
    renderPage(openProgram)
    expect(
      await screen.findByRole('button', { name: 'Get my referral link' })
    ).toHaveAttribute('href', '/referral')
  })

  it('updates the estimate from people, average top-up and the rate', async () => {
    const user = userEvent.setup()
    renderPage(openProgram)
    const people = await screen.findByLabelText('People you invite')
    await user.clear(people)
    await user.type(people, '20')
    const average = screen.getByLabelText('Average monthly top-up (USD)')
    await user.clear(average)
    await user.type(average, '100')
    expect(screen.getByTestId('affiliate-estimate')).toHaveTextContent(
      '$240.00'
    )
  })
})

describe('estimateCommission', () => {
  it('multiplies people, average top-up and the percent rate', () => {
    expect(estimateCommission(10, 50, 12)).toBe(60)
    expect(estimateCommission(3, 33.33, 10)).toBe(10)
  })

  it('returns null for negative or non-numeric input', () => {
    expect(estimateCommission(-1, 50, 10)).toBeNull()
    expect(estimateCommission(Number.NaN, 50, 10)).toBeNull()
  })
})
