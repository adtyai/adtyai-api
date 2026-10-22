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
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { HeaderBalance } from '../header-balance'

const getSelf = vi.fn()
let walletVisible = true

vi.mock('@/lib/api', () => ({ getSelf: () => getSelf() }))
vi.mock('@/lib/format', () => ({
  formatQuota: (q: number) => `$${q / 500000}`,
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => walletVisible,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { to: string; children: ReactNode }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

function renderIt() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <HeaderBalance />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  getSelf.mockReset()
  walletVisible = true
  useAuthStore.getState().auth.reset()
})

describe('HeaderBalance', () => {
  it('renders nothing when signed out', () => {
    renderIt()
    expect(screen.queryByTestId('header-balance')).toBeNull()
    expect(getSelf).not.toHaveBeenCalled()
  })

  it('shows quota from /api/user/self and links recharge to /wallet', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'u', role: 1, quota: 500000 })
    getSelf.mockResolvedValue({ success: true, data: { quota: 1500000 } })
    renderIt()
    await waitFor(() => expect(screen.getByText('$3')).toBeTruthy())
    expect(screen.getByRole('link').getAttribute('href')).toBe('/wallet')
  })

  it('shows bonus and paid balance separately', async () => {
    useAuthStore
      .getState()
      .auth.setUser({
        id: 1,
        username: 'u',
        role: 1,
        quota: 500000,
        bonus_quota: 500000,
      })
    getSelf.mockResolvedValue({
      success: true,
      data: { quota: 1000000, bonus_quota: 500000 },
    })
    renderIt()
    await waitFor(() => expect(screen.getByText('$2')).toBeTruthy())
    expect(screen.getByText('$1')).toBeTruthy()
    expect(screen.queryByText('$3')).toBeNull()
  })

  it('keeps the stored bonus when self omits bonus_quota', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'u',
      role: 1,
      quota: 500000,
      bonus_quota: 500000,
    })
    getSelf.mockResolvedValue({
      success: true,
      data: { quota: 1000000 },
    })
    renderIt()
    await waitFor(() => expect(screen.getByText('$2')).toBeTruthy())
    expect(screen.getByText('$1')).toBeTruthy()
  })

  it('hides when the wallet module is hidden', () => {
    walletVisible = false
    useAuthStore.getState().auth.setUser({ id: 1, username: 'u', role: 1 })
    renderIt()
    expect(screen.queryByTestId('header-balance')).toBeNull()
  })
})
