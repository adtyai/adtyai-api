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
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useSidebarView } from '@/hooks/use-sidebar-view'
import type { PermissionCatalog } from '@/lib/admin-permissions'
import {
  ADTY_PERMISSION_PRESETS,
  applyPermissionPreset,
  canViewAllLogs,
  type AdtyPermissionPreset,
} from '@/lib/adty-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

beforeEach(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  useAuthStore.getState().auth.reset()
})

async function adminMenuFor(
  role: number,
  adminPermissions?: Record<string, Record<string, boolean>>
) {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: 'staff',
    role,
    permissions: adminPermissions
      ? { admin_permissions: adminPermissions }
      : undefined,
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], { SidebarModulesAdmin: '' })
  let groups: ReturnType<typeof useSidebarView>['navGroups'] = []
  function Probe() {
    groups = useSidebarView().navGroups
    return null
  }
  const router = createRouter({
    routeTree: createRootRoute({ component: Probe }),
    history: createMemoryHistory({ initialEntries: ['/dashboard/overview'] }),
  })
  await router.load()
  function Wrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
  }
  render(<RouterProvider router={router} />, { wrapper: Wrapper })
  await waitFor(() => expect(groups.length).toBeGreaterThan(0))
  return groups.find((group) => group.id === 'admin')
}

describe('permission-aware admin menu', () => {
  it('hides the admin group from an admin without permissions', async () => {
    expect(await adminMenuFor(ROLE.ADMIN)).toBeUndefined()
  })

  it('shows only Users to an admin who can view users', async () => {
    const admin = await adminMenuFor(ROLE.ADMIN, { adty_user: { read: true } })
    expect(admin?.items.map((item) => item.title)).toEqual(['Users'])
  })

  it('shows every admin entry to root', async () => {
    const admin = await adminMenuFor(ROLE.SUPER_ADMIN)
    expect(admin?.items.map((item) => item.title)).toEqual(
      expect.arrayContaining([
        'Channels',
        'Models',
        'Users',
        'System Info',
        'System Settings',
      ])
    )
  })
})

describe('site-wide log access', () => {
  it('needs the log permission for admins and is always on for root', () => {
    const admin = { id: 2, username: 'a', role: ROLE.ADMIN }
    expect(canViewAllLogs(admin)).toBe(false)
    expect(
      canViewAllLogs({
        ...admin,
        permissions: { admin_permissions: { adty_log: { read: true } } },
      })
    ).toBe(true)
    expect(
      canViewAllLogs({ id: 1, username: 'r', role: ROLE.SUPER_ADMIN })
    ).toBe(true)
    expect(canViewAllLogs({ id: 3, username: 'u', role: ROLE.USER })).toBe(
      false
    )
  })
})

describe('permission presets', () => {
  const catalog: PermissionCatalog = {
    roles: [],
    resources: [
      {
        resource: 'adty_user',
        label_key: 'Users',
        actions: ['read', 'write', 'group', 'quota', 'delete'].map(
          (action) => ({
            action,
            label_key: action,
            description_key: action,
          })
        ),
      },
      {
        resource: 'adty_log',
        label_key: 'Logs',
        actions: [{ action: 'read', label_key: 'r', description_key: 'r' }],
      },
      {
        resource: 'adty_finance',
        label_key: 'Finance',
        actions: ['read', 'manual_credit', 'refund'].map((action) => ({
          action,
          label_key: action,
          description_key: action,
        })),
      },
      {
        resource: 'adty_referral',
        label_key: 'Referrals',
        actions: ['read', 'review', 'payout', 'adjust', 'settings'].map(
          (action) => ({ action, label_key: action, description_key: action })
        ),
      },
      {
        resource: 'channel',
        label_key: 'Channels',
        actions: [{ action: 'read', label_key: 'r', description_key: 'r' }],
      },
    ],
  }
  const preset = (id: AdtyPermissionPreset['id']): AdtyPermissionPreset => {
    const found = ADTY_PERMISSION_PRESETS.find((p) => p.id === id)
    if (!found) throw new Error(`missing preset ${id}`)
    return found
  }
  const countOn = (matrix: Record<string, Record<string, boolean>>) =>
    Object.values(matrix).flatMap((actions) =>
      Object.values(actions).filter(Boolean)
    ).length

  it('applying Support turns on exactly its four permissions', () => {
    const matrix = applyPermissionPreset(catalog, preset('support'))
    expect(countOn(matrix)).toBe(4)
    expect(matrix.adty_user.read).toBe(true)
    expect(matrix.adty_log.read).toBe(true)
    expect(matrix.adty_finance.read).toBe(true)
    expect(matrix.adty_referral.read).toBe(true)
    expect(matrix.adty_user.quota).toBe(false)
  })

  it('a preset replaces earlier grants instead of adding to them', () => {
    const matrix = applyPermissionPreset(catalog, preset('finance'))
    expect(matrix.adty_log.read).toBe(false)
    expect(matrix.channel.read).toBe(false)
    expect(matrix.adty_user.quota).toBe(true)
    expect(matrix.adty_finance.refund).toBe(true)
    expect(matrix.adty_referral.settings).toBe(false)
  })
})
