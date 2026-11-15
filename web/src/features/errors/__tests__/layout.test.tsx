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
import { describe, expect, it } from 'vitest'

import { NotFoundError } from '../not-found-error'

async function renderNotFound() {
  const root = createRootRoute()
  const notFound = createRoute({
    getParentRoute: () => root,
    path: '/404',
    component: NotFoundError,
  })
  const router = createRouter({
    routeTree: root.addChildren([notFound]),
    history: createMemoryHistory({ initialEntries: ['/404'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
}

describe('error page layout', () => {
  it('sets the error code as a 120px faint watermark behind the message', async () => {
    await renderNotFound()

    const code = await screen.findByRole('heading', { name: '404' })
    expect(code).toHaveClass('text-faint/40', 'text-[120px]', 'font-extrabold')
  })

  it('keeps a secondary button beside the brand primary action', async () => {
    await renderNotFound()

    expect(await screen.findByRole('button', { name: 'Go Back' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Back to Home' })).toBeVisible()
  })
})
