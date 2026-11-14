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

import { TeamPanel } from '../team-panel'

function mockMineTeamGet(handler: () => unknown | Promise<unknown>) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url)
    if (path === '/api/adty/teams/mine') {
      return handler()
    }
    return { data: { success: true, data: { items: [] } } }
  })
}

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('TeamPanel', () => {
  it('shows a loading state while the mine query is pending and does not offer create or join', async () => {
    mockMineTeamGet(() => new Promise(() => {}))
    render(<TeamPanel />, { wrapper })
    expect(await screen.findByText('Team')).toBeInTheDocument()
    expect(screen.getByText('Loading...')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Create team' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Join team' })).not.toBeInTheDocument()
  })

  it('shows an error state when the mine query fails and does not offer create or join', async () => {
    mockMineTeamGet(() => {
      throw new Error('offline')
    })
    render(<TeamPanel />, { wrapper })
    expect(
      await screen.findByText('Oops! Something went wrong')
    ).toBeInTheDocument()
    expect(screen.getByText('Team')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Create team' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Join team' })).not.toBeInTheDocument()
  })

  it('retries the mine query until create and join appear when the user has no team', async () => {
    let failed = true
    mockMineTeamGet(() => {
      if (failed) throw new Error('offline')
      return { data: { success: true, data: { team: null } } }
    })
    render(<TeamPanel />, { wrapper })
    const retry = await screen.findByRole('button', { name: 'Retry' })
    failed = false
    await userEvent.setup().click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
    )
    expect(await screen.findByRole('button', { name: 'Create team' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Join team' })).toBeInTheDocument()
  })

  it('shows team details instead of create or join when the user has a team', async () => {
    mockMineTeamGet(() => ({
      data: {
        success: true,
        data: {
          team: {
            team_id: 3,
            name: 'Alpha',
            role: 'member',
            discount: '1',
            shared_quota: 10,
            status: 'active',
          },
        },
      },
    }))
    render(<TeamPanel />, { wrapper })
    expect(await screen.findByText('Alpha')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Create team' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Join team' })).not.toBeInTheDocument()
  })
})
