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
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'

import { NavGroup } from '../nav-group'

const clients: QueryClient[] = []
const originalWidth = window.innerWidth

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  window.innerWidth = originalWidth
})

async function renderSubmenus(path: string, width: number) {
  window.innerWidth = width
  const submitted = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], {
    chats: [{ 'Team chat': 'https://chat.example.com' }],
  })
  const root = createRootRoute({
    component: () => (
      <form
        onSubmit={(event) => {
          event.preventDefault()
          submitted()
        }}
      >
        <NavGroup
          title='Console'
          items={[
            {
              title: 'Models',
              items: [{ title: 'Model metadata', url: '/models' }],
            },
            { title: 'Chat presets', type: 'chat-presets' },
          ]}
        />
      </form>
    ),
  })
  const route = createRoute({ getParentRoute: () => root, path: '$' })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <SidebarProvider>
        <RouterProvider router={router} />
      </SidebarProvider>
    </QueryClientProvider>
  )
  return { router, submitted, user: userEvent.setup() }
}

describe.each([1024, 375])('submenu collapse at width %i', (width) => {
  it('keeps the active parent folded until its label or arrow is clicked', async () => {
    const { router, submitted, user } = await renderSubmenus('/models', width)
    const trigger = screen.getByRole('button', { name: 'Models' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('link', { name: 'Model metadata' })).toBeNull()

    await user.click(screen.getByText('Models'))
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.getByRole('link', { name: 'Model metadata' })
    ).toHaveAttribute('data-active')
    const arrow = trigger.querySelector('svg')
    if (!arrow) throw new Error('Submenu toggle arrow is missing')
    await user.click(arrow)
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(router.state.location.pathname).toBe('/models')
    expect(submitted).not.toHaveBeenCalled()
  })

  it('keeps active chat presets folded and supports keyboard and arrow toggles', async () => {
    const { router, submitted, user } = await renderSubmenus('/chat/0', width)
    const trigger = screen.getByRole('button', { name: 'Chat presets' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('link', { name: 'Team chat' })).toHaveAttribute(
      'data-active'
    )
    const arrow = trigger.querySelector('svg')
    if (!arrow) throw new Error('Chat toggle arrow is missing')
    await user.click(arrow)
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(router.state.location.pathname).toBe('/chat/0')
    expect(submitted).not.toHaveBeenCalled()
  })

  it('preserves the chosen fold state when the active route changes', async () => {
    const { router, user } = await renderSubmenus('/users', width)
    const trigger = screen.getByRole('button', { name: 'Models' })
    await act(async () => {
      await router.navigate({ to: '/models' })
    })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    await user.click(trigger)
    await act(async () => {
      await router.navigate({ to: '/users' })
    })
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
  })
})
