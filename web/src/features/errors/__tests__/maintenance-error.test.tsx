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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { MaintenanceError } from '../maintenance-error'

async function renderMaintenance() {
  const root = createRootRoute()
  const status = createRoute({
    getParentRoute: () => root,
    path: '/status',
    component: () => <p>status page</p>,
  })
  const maintenance = createRoute({
    getParentRoute: () => root,
    path: '/503',
    component: MaintenanceError,
  })
  const router = createRouter({
    routeTree: root.addChildren([status, maintenance]),
    history: createMemoryHistory({ initialEntries: ['/503'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  return router
}

describe('maintenance error', () => {
  it('sends Learn more to the status page', async () => {
    const router = await renderMaintenance()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: 'Learn more' }))

    expect(router.state.location.pathname).toBe('/status')
  })
})
