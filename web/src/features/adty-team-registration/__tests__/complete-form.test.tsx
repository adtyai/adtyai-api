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

import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { TeamRegistrationCompleteForm } from '../components/team-registration-complete-form'

const teamApi = vi.hoisted(() => ({
  completeTeamRegistration: vi.fn(),
}))

vi.mock('../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api')>()
  return {
    ...actual,
    completeTeamRegistration: teamApi.completeTeamRegistration,
  }
})

async function renderForm(code: string) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => <TeamRegistrationCompleteForm code={code} />,
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
}

async function fillHappyPath(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Username'), 'teamlead')
  await user.type(screen.getByLabelText('Password'), 'password-123')
  await user.type(screen.getByLabelText('Confirm password'), 'password-123')
  await user.type(screen.getByLabelText('Team name'), 'Alpha team')
}

beforeEach(() => {
  teamApi.completeTeamRegistration.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('TeamRegistrationCompleteForm', () => {
  it('completes registration through a valid link', async () => {
    const user = userEvent.setup()
    teamApi.completeTeamRegistration.mockResolvedValue({
      Team: { id: 8, name: 'Alpha team' },
    })
    await renderForm('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
    await fillHappyPath(user)
    await user.click(screen.getByRole('button', { name: 'Register team' }))
    await waitFor(() =>
      expect(teamApi.completeTeamRegistration).toHaveBeenCalledWith({
        code: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        username: 'teamlead',
        password: 'password-123',
        name: 'Alpha team',
      })
    )
    expect(
      await screen.findByText(
        'Your team has been created. You can sign in with this username.'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Team name: Alpha team')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/sign-in'
    )
  })

  it('shows the backend error for a revoked link', async () => {
    const user = userEvent.setup()
    teamApi.completeTeamRegistration.mockRejectedValue(
      new Error('This team registration link has been revoked')
    )
    await renderForm('bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb')
    await fillHappyPath(user)
    await user.click(screen.getByRole('button', { name: 'Register team' }))
    expect(
      await screen.findByText('This team registration link has been revoked')
    ).toBeInTheDocument()
    expect(teamApi.completeTeamRegistration).toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Register team' })
    ).toBeInTheDocument()
  })

  it('rejects an invalid link without calling the complete API', async () => {
    await renderForm('short')
    expect(
      await screen.findByText('This team registration link is invalid')
    ).toBeInTheDocument()
    expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
    expect(teamApi.completeTeamRegistration).not.toHaveBeenCalled()
  })
})
