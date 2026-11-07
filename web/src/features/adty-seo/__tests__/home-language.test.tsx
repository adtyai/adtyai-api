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
import { act, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useReferralCapture } from '@/features/referral/lib/click'
import { Route as LangHomeRoute } from '@/routes/$lang'
import { Route as RootHomeRoute } from '@/routes/index'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { HOME_LANG_STORAGE_KEY } from '../home-langs'
import { useAdtyAfterLanguageChange } from '../home-language'
import { LanguageHint } from '../language-hint'

const referralApi = vi.hoisted(() => ({ recordReferralClick: vi.fn() }))
vi.mock('@/features/referral/api', () => referralApi)

// The real routes are typed against the whole route tree; the test mounts
// their beforeLoad on a small tree of its own.
type RootHomeBeforeLoad = (context: {
  location: { searchStr: string; search: Record<string, unknown> }
}) => Promise<void>
type LangHomeBeforeLoad = (context: {
  params: { lang: string }
}) => Promise<void>

let languageSwitch: ((code: string) => void) | undefined

function HomeStub() {
  // Mirrors the language switcher: change the language, then let the home
  // page follow it.
  const afterChange = useAdtyAfterLanguageChange()
  languageSwitch = (code) => {
    void i18next.changeLanguage(code).then(() => afterChange(code))
  }
  return <p>home {i18next.language}</p>
}

function PricingStub() {
  const afterChange = useAdtyAfterLanguageChange()
  languageSwitch = (code) => {
    void i18next.changeLanguage(code).then(() => afterChange(code))
  }
  return <p>pricing</p>
}

function RootStub() {
  useReferralCapture()
  return <Outlet />
}

function renderAt(url: string) {
  const root = createRootRoute({
    component: RootStub,
    notFoundComponent: () => <p>not found</p>,
  })
  const routes = [
    createRoute({
      getParentRoute: () => root,
      path: '/',
      beforeLoad: RootHomeRoute.options
        .beforeLoad as unknown as RootHomeBeforeLoad,
      component: HomeStub,
    }),
    createRoute({
      getParentRoute: () => root,
      path: '/pricing',
      component: PricingStub,
    }),
    createRoute({
      getParentRoute: () => root,
      path: '/$lang',
      beforeLoad: LangHomeRoute.options
        .beforeLoad as unknown as LangHomeBeforeLoad,
      component: HomeStub,
    }),
  ]
  const router = createRouter({
    routeTree: root.addChildren(routes),
    history: createMemoryHistory({ initialEntries: [url] }),
    defaultPreload: 'intent',
    defaultPreloadStaleTime: 0,
  })
  render(<RouterProvider router={router} />)
  return router
}

function setBrowserLanguages(languages: string[]) {
  vi.spyOn(window.navigator, 'languages', 'get').mockReturnValue(languages)
}

beforeEach(async () => {
  window.localStorage.clear()
  referralApi.recordReferralClick.mockResolvedValue(undefined)
  await i18next.changeLanguage('en')
})

afterEach(async () => {
  useAuthStore.getState().auth.reset()
  languageSwitch = undefined
  await i18next.changeLanguage('en')
})

describe('home language URLs', () => {
  it('switches to Simplified Chinese on /zh-cn and remembers it', async () => {
    renderAt('/zh-cn')

    await screen.findByText('home zhCN')
    expect(window.localStorage.getItem(HOME_LANG_STORAGE_KEY)).toBe('zhCN')
  })

  it('sends / to /zh-cn when Chinese was picked before', async () => {
    window.localStorage.setItem(HOME_LANG_STORAGE_KEY, 'zhCN')
    const router = renderAt('/')

    await screen.findByText('home zhCN')
    expect(router.state.location.pathname).toBe('/zh-cn')
  })

  it.each([null, 100])(
    'preloads / from the Chinese home without blocking navigation (role: %s)',
    async (role) => {
      if (role !== null) {
        useAuthStore
          .getState()
          .auth.setUser({ id: 1, username: 'u', role } as AuthUser)
      }
      const router = renderAt('/zh-cn?utm=stay')
      await screen.findByText('home zhCN')

      // Bound recursion so a regression fails instead of starving timers and
      // exhausting the test worker's heap. All preloads use the real router.
      const preloadRoute = router.preloadRoute
      let preloads = 0
      vi.spyOn(router, 'preloadRoute').mockImplementation((options) => {
        if (++preloads > 8) throw new Error('Home preload redirect loop')
        return preloadRoute(options)
      })

      await act(async () => {
        const matches = await router.preloadRoute({
          to: '/',
          search: { utm: 'keep', ref: 'abc' },
        })
        expect(matches?.at(-1)).toMatchObject({
          pathname: '/zh-cn',
          status: 'success',
          search: { utm: 'keep', ref: 'abc' },
        })
      })

      expect(router.state.location.href).toBe('/zh-cn?utm=stay')
      expect(window.localStorage.getItem(HOME_LANG_STORAGE_KEY)).toBe('zhCN')
      expect(referralApi.recordReferralClick).not.toHaveBeenCalled()

      act(() => languageSwitch?.('en'))
      await waitFor(() => expect(router.state.location.href).toBe('/?utm=stay'))
      expect(await screen.findByText('home en')).toBeInTheDocument()
    }
  )

  it('keeps ?ref= on the redirect and records the click once', async () => {
    window.localStorage.setItem(HOME_LANG_STORAGE_KEY, 'zhCN')
    const router = renderAt('/?ref=abc')

    await waitFor(() =>
      expect(referralApi.recordReferralClick).toHaveBeenCalledWith(
        'abc',
        '/zh-cn'
      )
    )
    await waitFor(() => expect(router.state.location.searchStr).toBe(''))
    expect(router.state.location.pathname).toBe('/zh-cn')
    expect(referralApi.recordReferralClick).toHaveBeenCalledTimes(1)
  })

  it('stays on / in English when no language was picked', async () => {
    await i18next.changeLanguage('ja')
    const router = renderAt('/')

    await screen.findByText('home en')
    expect(router.state.location.pathname).toBe('/')
  })

  it('sends a signed-in user with a Chinese interface from / to /zh-cn', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'u', role: 1 } as AuthUser)
    await i18next.changeLanguage('zhCN')
    const router = renderAt('/')

    await screen.findByText('home zhCN')
    expect(router.state.location.pathname).toBe('/zh-cn')
  })

  it('shows not found for a segment that is not a home language', async () => {
    renderAt('/xx')

    expect(await screen.findByText('not found')).toBeInTheDocument()
    expect(window.localStorage.getItem(HOME_LANG_STORAGE_KEY)).toBeNull()
  })

  it('keeps the query when the home language changes', async () => {
    const router = renderAt('/?utm=keep')
    await screen.findByText('home en')

    act(() => languageSwitch?.('ko'))

    await waitFor(() => expect(router.state.location.pathname).toBe('/ko'))
    expect(router.state.location.searchStr).toBe('?utm=keep')
  })

  it('moves to the new language URL when the language changes on the home page', async () => {
    const router = renderAt('/')
    await screen.findByText('home en')

    act(() => languageSwitch?.('ja'))

    await waitFor(() => expect(router.state.location.pathname).toBe('/ja'))
    expect(window.localStorage.getItem(HOME_LANG_STORAGE_KEY)).toBe('ja')
  })

  it('keeps the address when the language changes on another page', async () => {
    const router = renderAt('/pricing')
    await screen.findByText('pricing')

    act(() => languageSwitch?.('ja'))

    await waitFor(() => expect(i18next.language).toBe('ja'))
    expect(router.state.location.pathname).toBe('/pricing')
    expect(window.localStorage.getItem(HOME_LANG_STORAGE_KEY)).toBeNull()
  })
})

describe('language hint', () => {
  it('suggests Chinese to a first-time visitor with a Chinese browser', async () => {
    setBrowserLanguages(['zh-CN', 'en'])
    render(<LanguageHint />)

    const link = await screen.findByRole('link', { name: /简体中文/ })
    expect(link).toHaveAttribute('href', '/zh-cn')
  })

  it('stays hidden for an English browser', async () => {
    setBrowserLanguages(['en-US', 'zh-CN'])
    render(<LanguageHint />)

    await act(async () => {})
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('stays hidden once a home language was picked', async () => {
    setBrowserLanguages(['zh-CN'])
    window.localStorage.setItem(HOME_LANG_STORAGE_KEY, 'en')
    render(<LanguageHint />)

    await act(async () => {})
    expect(screen.queryByRole('status')).toBeNull()
  })
})
