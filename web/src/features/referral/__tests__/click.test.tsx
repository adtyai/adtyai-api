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
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ensureDeviceCookie, useReferralCapture } from '../lib/click'

const api = vi.hoisted(() => ({ recordReferralClick: vi.fn() }))
vi.mock('../api', () => api)

afterEach(() => {
  api.recordReferralClick.mockReset()
  document.cookie = 'adty_did=; Max-Age=0; Path=/'
})

function Root() {
  useReferralCapture()
  return <Outlet />
}

function renderAt(url: string) {
  const rootRoute = createRootRoute({ component: Root })
  const pricing = createRoute({
    getParentRoute: () => rootRoute,
    path: '/pricing',
    component: () => <p>pricing</p>,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([pricing]),
    history: createMemoryHistory({ initialEntries: [url] }),
  })
  render(<RouterProvider router={router} />)
  return router
}

describe('useReferralCapture', () => {
  it('records ?ref= and removes it from the address while keeping other params', async () => {
    api.recordReferralClick.mockResolvedValue(undefined)
    const router = renderAt('/pricing?ref=ABCD2345&utm=x')
    await waitFor(() =>
      expect(api.recordReferralClick).toHaveBeenCalledWith(
        'ABCD2345',
        '/pricing'
      )
    )
    await waitFor(() =>
      expect(router.state.location.search).toEqual({ utm: 'x' })
    )
  })

  it('ignores a failed click', async () => {
    api.recordReferralClick.mockRejectedValue(new Error('invalid'))
    const router = renderAt('/pricing?ref=NOPE')
    await waitFor(() => expect(router.state.location.search).toEqual({}))
  })
})

describe('ensureDeviceCookie', () => {
  it('writes the device cookie once', () => {
    ensureDeviceCookie()
    const first = document.cookie
    expect(first).toMatch(/adty_did=[0-9a-f]{32}/)
    ensureDeviceCookie()
    expect(document.cookie).toBe(first)
  })
})
