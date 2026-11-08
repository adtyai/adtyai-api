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
  lazyRouteComponent,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  render as renderView,
  screen,
  waitFor,
} from '@testing-library/react'
import { Suspense, type ReactElement } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { lazyRouteComponent as recoveredLazyRouteComponent } from '@/lib/route-lazy-component'

import { GeneralError } from '../general-error'

const reload = vi.fn()

// ADTY task 8.9: the report button reads the support address from
// GET /api/adty/site.
const siteApi = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: new Proxy(actual.api, {
      get(target, property) {
        if (property === 'get') return siteApi.get
        return Reflect.get(target, property, target)
      },
    }),
  }
})

function render(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return renderView(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>
  )
}

beforeEach(() => {
  siteApi.get.mockResolvedValue({ data: { success: true, data: {} } })
  sessionStorage.clear()
  const realWindow = window
  vi.stubGlobal(
    'window',
    new Proxy(realWindow, {
      get(target, property) {
        if (property === 'location') return { ...target.location, reload }
        return Reflect.get(target, property, target)
      },
    })
  )
})

afterEach(() => {
  siteApi.get.mockReset()
  vi.unstubAllGlobals()
  sessionStorage.clear()
})

function createTestRouter(
  error?: Error | { response: { status: number } },
  useRecoveredLazy = false
) {
  const root = createRootRoute({
    component: Outlet,
    errorComponent: GeneralError,
  })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <p>Current page</p>,
  })
  const importRoute = () =>
    error
      ? Promise.reject(error)
      : Promise.resolve({ default: () => <p>Recovered route</p> })
  const broken = createRoute({
    getParentRoute: () => root,
    path: '/wallet',
    component: useRecoveredLazy
      ? recoveredLazyRouteComponent(importRoute)
      : lazyRouteComponent(importRoute),
  })
  return createRouter({
    routeTree: root.addChildren([home, broken]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
    defaultPreload: 'intent',
  })
}

test.each([
  Object.assign(
    new Error('Loading chunk 42 failed. (error: /static/js/async/42.old.js)'),
    { name: 'ChunkLoadError' }
  ),
  Object.assign(
    new Error('Loading CSS chunk 42 failed. (/static/css/42.old.css)'),
    { code: 'CSS_CHUNK_LOAD_FAILED' }
  ),
])(
  'a failed route chunk reloads once, then displays an error without a fake HTTP status: $message',
  async (error) => {
    const router = createTestRouter(error)
    const view = render(<RouterProvider router={router} />)
    await screen.findByText('Current page')
    await act(() => router.navigate({ to: '/wallet' }))
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
    view.unmount()

    const afterReload = createTestRouter(error)
    render(<RouterProvider router={afterReload} />)
    await screen.findByText('Current page')
    await act(() => afterReload.navigate({ to: '/wallet' }))
    expect(await screen.findByText(/Oops! Something went wrong/)).toBeVisible()
    expect(screen.queryByText('500')).not.toBeInTheDocument()
    expect(reload).toHaveBeenCalledTimes(1)
  }
)

test.each([
  new TypeError(
    'Failed to fetch dynamically imported module: https://example.com/42.js'
  ),
  new TypeError(
    'error loading dynamically imported module: https://example.com/42.js'
  ),
  new TypeError('Importing a module script failed.'),
])(
  'a native project route import reloads once, then shows its error: $message',
  async (error) => {
    const router = createTestRouter(error, true)
    const view = render(<RouterProvider router={router} />)
    await screen.findByText('Current page')
    await act(() => router.navigate({ to: '/wallet' }))
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
    view.unmount()

    const afterReload = createTestRouter(error, true)
    render(<RouterProvider router={afterReload} />)
    await screen.findByText('Current page')
    await act(() => afterReload.navigate({ to: '/wallet' }))
    expect(await screen.findByText(/Oops! Something went wrong/)).toBeVisible()
    expect(reload).toHaveBeenCalledTimes(1)
  }
)

test('failed native intent preloads leave the current screen and recovery marker untouched until navigation', async () => {
  const error = new TypeError(
    'Failed to fetch dynamically imported module: https://example.com/preload.js'
  )
  const router = createTestRouter(error, true)
  render(<RouterProvider router={router} />)
  await screen.findByText('Current page')

  await act(() => router.preloadRoute({ to: '/wallet' }))
  await act(() => router.preloadRoute({ to: '/wallet' }))

  expect(screen.getByText('Current page')).toBeVisible()
  expect(reload).not.toHaveBeenCalled()
  expect(
    sessionStorage.getItem(`tanstack_router_reload:${error.message}`)
  ).toBeNull()

  await act(() => router.navigate({ to: '/wallet' }))
  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
})

test('the project route helper preserves named exports after repeated preload', async () => {
  const importer = vi.fn(async () => ({
    Screen: () => <p>Named route ready</p>,
  }))
  const Screen = recoveredLazyRouteComponent(importer, 'Screen')

  await Promise.all([Screen.preload?.(), Screen.preload?.()])
  render(
    <Suspense fallback={<p>Pending</p>}>
      <Screen />
    </Suspense>
  )

  expect(await screen.findByText('Named route ready')).toBeVisible()
  expect(importer).toHaveBeenCalledTimes(1)
  expect(reload).not.toHaveBeenCalled()
})

test.each(['getter', 'getItem', 'setItem'] as const)(
  'a native route failure with blocked recovery storage %s shows the error without reloading',
  async (operation) => {
    const blocked = () => {
      throw new DOMException('Recovery storage is blocked', 'SecurityError')
    }
    vi.stubGlobal('sessionStorage', {
      getItem: operation === 'getItem' ? blocked : () => null,
      setItem: operation === 'setItem' ? blocked : () => undefined,
    })
    if (operation === 'getter') {
      Object.defineProperty(globalThis, 'sessionStorage', {
        configurable: true,
        get: blocked,
      })
    }
    const error = new TypeError(
      'Failed to fetch dynamically imported module: https://example.com/blocked.js'
    )
    const router = createTestRouter(error, true)
    render(<RouterProvider router={router} />)
    await screen.findByText('Current page')

    await act(() => router.navigate({ to: '/wallet' }))

    expect(await screen.findByText(/Oops! Something went wrong/)).toBeVisible()
    expect(reload).not.toHaveBeenCalled()
  }
)

test('a native module load failure recovers when the route loads after the guarded reload', async () => {
  const error = new TypeError(
    'Failed to fetch dynamically imported module: https://example.com/45.js'
  )
  const router = createTestRouter(error, true)
  const view = render(<RouterProvider router={router} />)
  await screen.findByText('Current page')
  await act(() => router.navigate({ to: '/wallet' }))
  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
  view.unmount()

  const recovered = createTestRouter(undefined, true)
  render(<RouterProvider router={recovered} />)
  await screen.findByText('Current page')
  await act(() => recovered.navigate({ to: '/wallet' }))
  expect(await screen.findByText('Recovered route')).toBeVisible()
  expect(reload).toHaveBeenCalledTimes(1)
})

test.each([
  new TypeError('Cannot read properties of undefined'),
  new TypeError(
    'Business operation: Failed to fetch dynamically imported module'
  ),
  new TypeError('Failed to fetch dynamically imported module: user profile'),
  new Error(
    'Failed to fetch dynamically imported module: https://example.com/not-a-chunk'
  ),
])(
  'a business error does not trigger module recovery: $message',
  async (error) => {
    const router = createTestRouter(error, true)
    render(<RouterProvider router={router} />)
    await screen.findByText('Current page')
    await act(() => router.navigate({ to: '/wallet' }))
    expect(await screen.findByText(/Oops! Something went wrong/)).toBeVisible()
    expect(reload).not.toHaveBeenCalled()
  }
)

test('failed and repeated intent preloads keep the current page usable until navigation', async () => {
  const error = Object.assign(
    new Error('Loading chunk 43 failed. (missing: /43.js)'),
    { name: 'ChunkLoadError' }
  )
  const router = createTestRouter(error)
  render(<RouterProvider router={router} />)
  await screen.findByText('Current page')
  await act(() => router.preloadRoute({ to: '/wallet' }))
  await act(() => router.preloadRoute({ to: '/wallet' }))
  expect(screen.getByText('Current page')).toBeVisible()
  expect(reload).not.toHaveBeenCalled()
  expect(
    screen.queryByText(/Oops! Something went wrong/)
  ).not.toBeInTheDocument()
  await act(() => router.navigate({ to: '/wallet' }))
  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
})

test.each([undefined, 429, 500])(
  'non-chunk errors preserve their actual HTTP status (%s) without reloading',
  async (status) => {
    const error = status ? { response: { status } } : new Error('Render failed')
    const router = createTestRouter(error)
    render(<RouterProvider router={router} />)
    await screen.findByText('Current page')
    await act(() => router.navigate({ to: '/wallet' }))
    if (status) {
      expect(
        await screen.findByRole('heading', { name: String(status) })
      ).toBeVisible()
    } else {
      expect(screen.queryByText('500')).not.toBeInTheDocument()
    }
    if (status === 429) {
      expect(screen.getByText('Too many requests')).toBeVisible()
      expect(
        screen.getByText(/Please wait a moment before trying again/)
      ).toBeVisible()
    }
    expect(reload).not.toHaveBeenCalled()
  }
)

test('a chunk failure with blocked session storage shows the error without risking a reload loop', async () => {
  vi.stubGlobal('sessionStorage', {
    getItem: () => null,
    setItem: () => {
      throw new Error('Storage blocked')
    },
    clear: () => {},
  })
  const error = Object.assign(
    new Error('Loading chunk 44 failed. (error: /44.js)'),
    { name: 'ChunkLoadError' }
  )
  const router = createTestRouter(error)
  render(<RouterProvider router={router} />)
  await screen.findByText('Current page')
  await act(() => router.navigate({ to: '/wallet' }))
  expect(await screen.findByText(/Oops! Something went wrong/)).toBeVisible()
  expect(screen.queryByText('500')).not.toBeInTheDocument()
  expect(reload).not.toHaveBeenCalled()
})

test('the report button mails the configured support address', async () => {
  siteApi.get.mockResolvedValue({
    data: { success: true, data: { support_email: 'help@example.com' } },
  })
  const router = createTestRouter({ response: { status: 500 } })
  render(<RouterProvider router={router} />)
  await screen.findByText('Current page')
  await act(() => router.navigate({ to: '/wallet' }))

  const report = (await screen.findByText('Report an issue')).closest('a')
  expect(report).toHaveAttribute('href', 'mailto:help@example.com')
  expect(siteApi.get).toHaveBeenCalledWith('/api/adty/site')
})

test('the report button stays hidden without a support address', async () => {
  const router = createTestRouter({ response: { status: 500 } })
  render(<RouterProvider router={router} />)
  await screen.findByText('Current page')
  await act(() => router.navigate({ to: '/wallet' }))

  await screen.findByRole('heading', { name: '500' })
  await waitFor(() => expect(siteApi.get).toHaveBeenCalled())
  expect(screen.queryByText('Report an issue')).not.toBeInTheDocument()
  expect(document.querySelector('a[href*="github.com"]')).toBeNull()
})
