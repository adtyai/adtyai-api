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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { TeamRegistrationLinksPanel } from '../components/team-registration-links-panel'
import type { TeamRegistrationIssue, TeamRegistrationView } from '../api'

const teamApi = vi.hoisted(() => ({
  listTeamRegistrations: vi.fn(),
  issueTeamRegistration: vi.fn(),
  revokeTeamRegistration: vi.fn(),
}))

vi.mock('../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api')>()
  return {
    ...actual,
    listTeamRegistrations: teamApi.listTeamRegistrations,
    issueTeamRegistration: teamApi.issueTeamRegistration,
    revokeTeamRegistration: teamApi.revokeTeamRegistration,
  }
})

function view(
  overrides: Partial<TeamRegistrationView> = {}
): TeamRegistrationView {
  return {
    id: 11,
    site_id: 0,
    created_by: 1,
    created_at: 1_700_000_000,
    expires_at: 1_700_086_400,
    used_by: 0,
    used_at: 0,
    team_id: 0,
    revoked_by: 0,
    revoked_at: 0,
    status: 'outstanding',
    ...overrides,
  }
}

function issued(
  overrides: Partial<TeamRegistrationIssue> = {}
): TeamRegistrationIssue {
  return {
    id: 11,
    code: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    site_id: 0,
    expires_at: 1_700_086_400,
    created_by: 1,
    created_at: 1_700_000_000,
    status: 'outstanding',
    ...overrides,
  }
}

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

beforeEach(() => {
  teamApi.listTeamRegistrations.mockResolvedValue([])
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { origin: 'https://app.example' },
  })
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('TeamRegistrationLinksPanel', () => {
  it('issues a root registration link and shows the public URL once', async () => {
    const user = userEvent.setup()
    teamApi.issueTeamRegistration.mockResolvedValue(
      issued({ code: 'abc123def456ghi789jkl012mno345pq' })
    )
    render(<TeamRegistrationLinksPanel actor='root' />, { wrapper })
    expect(await screen.findByText('No registration links yet')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Issue registration link' }))
    await waitFor(() =>
      expect(teamApi.issueTeamRegistration).toHaveBeenCalledWith('root', 24 * 3600)
    )
    expect(
      await screen.findByText(
        'Registration link: https://app.example/team-register/abc123def456ghi789jkl012mno345pq'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('The registration link is shown once')).toBeInTheDocument()
    expect(teamApi.listTeamRegistrations).toHaveBeenCalledWith('root')
  })

  it('lets a site owner revoke an outstanding link', async () => {
    const user = userEvent.setup()
    teamApi.listTeamRegistrations.mockResolvedValue([view({ id: 22 })])
    teamApi.revokeTeamRegistration.mockResolvedValue(
      view({ id: 22, status: 'revoked', revoked_by: 9, revoked_at: 1_700_000_100 })
    )
    render(<TeamRegistrationLinksPanel actor='owner' />, { wrapper })
    expect(await screen.findByText(/#22/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Revoke registration link' }))
    await waitFor(() =>
      expect(teamApi.revokeTeamRegistration).toHaveBeenCalledWith('owner', 22)
    )
  })

  it('shows a backend error when issuing a link fails', async () => {
    const user = userEvent.setup()
    teamApi.issueTeamRegistration.mockRejectedValue(
      new Error('No permission to perform this action')
    )
    render(<TeamRegistrationLinksPanel actor='root' />, { wrapper })
    await screen.findByText('No registration links yet')
    await user.click(screen.getByRole('button', { name: 'Issue registration link' }))
    expect(
      await screen.findByText('No permission to perform this action')
    ).toBeInTheDocument()
  })
})
