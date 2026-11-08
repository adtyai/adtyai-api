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
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useState, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { ThemeProvider } from '@/context/theme-provider'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { TopNavLink } from '../../types'
import { MobileDrawer } from '../mobile-drawer'
import { NavLinkList } from '../nav-link-item'
import { PublicHeader } from '../public-header'
import { PublicNavigation } from '../public-navigation'
import { TopNav } from '../top-nav'

type NavigationCase = {
  name: string
  render: (links: TopNavLink[]) => ReactNode
  open?: () => Promise<unknown>
  query: (name: string) => HTMLElement[]
}

const clients: QueryClient[] = []
const initialSystemConfig = useSystemConfigStore.getState()
const allModulesHidden = {
  home: false,
  console: false,
  pricing: { enabled: false, requireAuth: false },
  rankings: { enabled: false, requireAuth: false },
  docs: false,
  about: false,
  affiliate: false,
}
const originalLinks: TopNavLink[] = [
  { id: 'custom:internal:first', title: 'Internal duplicate', href: '/same' },
  { id: 'custom:internal:second', title: 'Internal duplicate', href: '/same' },
  {
    id: 'custom:external:first',
    title: 'External duplicate',
    href: 'https://docs.example.com/same',
    external: true,
  },
  {
    id: 'custom:external:second',
    title: 'External duplicate',
    href: 'https://docs.example.com/same',
    external: true,
  },
]
const queryLinks = (name: string) => screen.getAllByRole('link', { name })
const publicHeader = (links: TopNavLink[]) => (
  <PublicHeader
    navLinks={links}
    showThemeSwitch={false}
    showLanguageSwitcher={false}
    showAuthButtons={false}
    showNotifications={false}
    logo={<span />}
    siteName='Navigation identity fixture'
  />
)
const navigationCases: NavigationCase[] = [
  {
    name: 'TopNav desktop',
    render: (links) => <TopNav links={links} />,
    query: queryLinks,
  },
  {
    name: 'TopNav mobile dropdown',
    render: (links) => <TopNav links={links} />,
    open: () => userEvent.click(screen.getByRole('button')),
    query: (name) => screen.getAllByRole('menuitem', { name }),
  },
  {
    name: 'PublicHeader desktop',
    render: publicHeader,
    query: queryLinks,
  },
  {
    name: 'PublicHeader mobile',
    render: publicHeader,
    open: () =>
      userEvent.click(
        screen.getByRole('button', { name: 'Toggle navigation menu' })
      ),
    query: (name) => {
      const navigation = screen.getAllByRole('navigation').at(-1)
      if (!navigation) throw new Error('Mobile navigation is not open')
      return within(navigation).getAllByRole('link', {
        name,
      })
    },
  },
  {
    name: 'PublicNavigation',
    render: (links) => <PublicNavigation links={links} />,
    query: queryLinks,
  },
  {
    name: 'NavLinkList',
    render: (links) => <NavLinkList links={links} />,
    query: queryLinks,
  },
  {
    name: 'MobileDrawer',
    render: (links) => (
      <MobileDrawer
        isOpen
        onClose={() => {}}
        homeUrl='/'
        displayLogo={<span />}
        displaySiteName='Navigation identity fixture'
        loading={false}
        logoLoaded
        mobileLinksList={links}
        showAuthButtons={false}
        user={null}
      />
    ),
    query: queryLinks,
  },
]

beforeEach(() => {
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.getState().setLoading(false)
  i18next.addResourceBundle('fr', 'translation', { Home: 'Accueil' })
})

afterEach(async () => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(initialSystemConfig)
  await i18next.changeLanguage('en')
  i18next.removeResourceBundle('fr', 'translation')
  localStorage.clear()
})

async function renderNavigation(navigation: NavigationCase, dynamic = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], {
    HeaderNavModules: dynamic ? {} : allModulesHidden,
  })
  client.setQueryData(['notice'], { success: true, data: '' })
  let updateLinks = (_links: TopNavLink[]) => {}
  function NavigationFixture() {
    const [links, setLinks] = useState(originalLinks)
    const dynamicLinks = useTopNavLinks()
    updateLinks = setLinks
    return navigation.render(dynamic ? dynamicLinks : links)
  }
  const router = createRouter({
    routeTree: createRootRoute({ component: NavigationFixture }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='light'>
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
  if (navigation.open) await navigation.open()
  return async (links: TopNavLink[]) => {
    await act(async () => {
      updateLinks(links.map((link) => ({ ...link })))
    })
  }
}

describe.each(navigationCases)('$name stable link identity', (navigation) => {
  it('keeps duplicate link focus and identity when a custom link is prepended', async () => {
    const updateLinks = await renderNavigation(navigation)
    const internal = navigation.query('Internal duplicate')[1]
    const external = navigation.query('External duplicate')[1]
    internal.focus()

    await updateLinks([
      { id: 'custom:prepended', title: 'Prepended', href: '/prepended' },
      ...originalLinks,
    ])

    expect(navigation.query('Internal duplicate')).toHaveLength(2)
    expect(navigation.query('Internal duplicate')[1]).toBe(internal)
    expect(navigation.query('External duplicate')).toHaveLength(2)
    expect(navigation.query('External duplicate')[1]).toBe(external)
    expect(internal).toHaveFocus()
  })

  it('keeps the surviving duplicate link focused when its sibling is deleted', async () => {
    const updateLinks = await renderNavigation(navigation)
    const internal = navigation.query('Internal duplicate')[1]
    const external = navigation.query('External duplicate')[1]
    internal.focus()

    await updateLinks([originalLinks[1], originalLinks[3]])

    expect(navigation.query('Internal duplicate')).toHaveLength(1)
    expect(navigation.query('Internal duplicate')[0]).toBe(internal)
    expect(navigation.query('External duplicate')).toHaveLength(1)
    expect(navigation.query('External duplicate')[0]).toBe(external)
    expect(internal).toHaveFocus()
  })

  it('moves the focused logical link when duplicate title and URL items reorder', async () => {
    const updateLinks = await renderNavigation(navigation)
    const internal = navigation.query('Internal duplicate')[1]
    const external = navigation.query('External duplicate')[1]
    internal.focus()

    await updateLinks([
      originalLinks[1],
      originalLinks[0],
      originalLinks[3],
      originalLinks[2],
    ])

    expect(navigation.query('Internal duplicate')).toHaveLength(2)
    expect(navigation.query('Internal duplicate')[0]).toBe(internal)
    expect(navigation.query('External duplicate')).toHaveLength(2)
    expect(navigation.query('External duplicate')[0]).toBe(external)
    expect(internal).toHaveFocus()
  })

  it('keeps focus on Home when the real navigation hook changes language', async () => {
    await renderNavigation(navigation, true)
    const home = navigation.query('Home')[0]
    home.focus()

    await act(async () => {
      await i18next.changeLanguage('fr')
    })

    expect(navigation.query('Accueil')[0]).toBe(home)
    expect(home).toHaveFocus()
  })
})
