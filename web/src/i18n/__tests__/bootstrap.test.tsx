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
import { createRequire } from 'node:module'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

import { act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ReactDOM, { type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

const createRoot = ReactDOM.createRoot
const roots: Root[] = []

beforeEach(() => {
  vi.resetModules()
  window.localStorage.clear()
  window.sessionStorage.clear()
  document.body.innerHTML = '<div id="root"></div>'
  vi.spyOn(ReactDOM, 'createRoot').mockImplementation((...args) => {
    const root = createRoot(...args)
    roots.push(root)
    return root
  })
  vi.spyOn(window.navigator, 'language', 'get').mockReturnValue('ja-JP')

  // Module reloads must not wrap the previous test's i18next.changeLanguage.
  vi.doMock('i18next', async (importOriginal) => {
    const actual = await importOriginal<typeof import('i18next')>()
    return { ...actual, default: actual.createInstance() }
  })

  // Keep this test on the startup boundary, without auth routes or HTTP.
  vi.doMock('@/lib/api', () => ({
    getStatus: async () => ({ success: true, data: {} }),
  }))
  vi.doMock('@/routeTree.gen', async () => {
    const { createRootRoute, createRoute, Outlet } =
      await import('@tanstack/react-router')
    const { useTranslation } = await import('react-i18next')
    const root = createRootRoute({ component: Outlet })
    const home = createRoute({
      getParentRoute: () => root,
      path: '/',
      component: function ReadyScreen() {
        const { t } = useTranslation()
        return <p>{t('Sign in')}</p>
      },
    })
    return { routeTree: root.addChildren([home]) }
  })
})

afterEach(async () => {
  await act(() => {
    for (const root of roots.splice(0)) root.unmount()
  })
  vi.restoreAllMocks()
  vi.doUnmock('@/i18n/locales/fr.json')
  vi.doUnmock('i18next')
  vi.doUnmock('@/lib/api')
  vi.doUnmock('@/routeTree.gen')
  vi.unstubAllGlobals()
  window.localStorage.clear()
  window.sessionStorage.clear()
  document.body.innerHTML = ''
})

test('the first screen uses the saved language after its real pack loads', async () => {
  window.localStorage.setItem('i18nextLng', 'fr')

  await act(async () => {
    await import('@/main')
  })

  expect(await screen.findByText('Se connecter')).toBeVisible()
  expect(window.localStorage.getItem('i18nextLng')).toBe('fr')
})

test.each(['getter', 'getItem'] as const)(
  'a blocked localStorage %s still mounts the browser language on first visit',
  async (operation) => {
    vi.spyOn(window.navigator, 'language', 'get').mockReturnValue('fr-FR')
    const blocked = () => {
      throw new DOMException('Storage is blocked', 'SecurityError')
    }
    if (operation === 'getter') {
      vi.spyOn(window, 'localStorage', 'get').mockImplementation(blocked)
    } else {
      vi.spyOn(Storage.prototype, 'getItem').mockImplementation(blocked)
    }

    await act(async () => {
      await import('@/main')
    })

    expect(await screen.findByText('Se connecter')).toBeVisible()
  }
)

test('a failed storage write keeps the selected language and first screen usable', async () => {
  window.localStorage.setItem('i18nextLng', 'fr')
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage is full', 'QuotaExceededError')
  })

  await act(async () => {
    await import('@/main')
  })

  expect(await screen.findByText('Se connecter')).toBeVisible()
})

test.each(['synchronous', 'asynchronous'] as const)(
  'a %s language startup failure displays Retry and recovers on the next attempt',
  async (failure) => {
    const user = userEvent.setup()
    window.localStorage.setItem('i18nextLng', 'fr')
    const config = await import('@/i18n/config')
    const ready = vi.spyOn(config, 'whenInterfaceLanguageReady')
    if (failure === 'synchronous') {
      ready.mockImplementationOnce(() => {
        throw new Error('Language startup failed')
      })
    } else {
      ready.mockRejectedValueOnce(new Error('Language startup failed'))
    }

    await act(async () => {
      await import('@/main')
    })

    const retry = await screen.findByRole('button', { name: 'Retry' })
    expect(retry).toBeVisible()
    await user.click(retry)
    expect(await screen.findByText('Se connecter')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
  }
)

test('a rejected language chunk shows Retry and preserves the choice until loading recovers', async () => {
  const user = userEvent.setup()
  window.localStorage.setItem('i18nextLng', 'fr')
  vi.doMock('@/i18n/locales/fr.json', () => {
    throw new TypeError('Failed to fetch dynamically imported module: /fr.json')
  })

  await act(async () => {
    await import('@/main')
  })

  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(retry).toBeVisible()
  expect(window.localStorage.getItem('i18nextLng')).toBe('fr')
  await user.click(retry)
  expect(await screen.findByRole('button', { name: 'Retry' })).toBeVisible()
  expect(window.localStorage.getItem('i18nextLng')).toBe('fr')
  vi.doUnmock('@/i18n/locales/fr.json')
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Se connecter')).toBeVisible()
})

test('the production route compiler preserves split paths and named exports while using the project recovery helper', async () => {
  const browserWindow = window
  const browserDocument = document
  // The compiler is Node tooling; its import-meta shim must not see jsdom.
  vi.stubGlobal('window', undefined)
  vi.stubGlobal('document', undefined)
  try {
    const { routeLazyRecoveryCompilerPlugin } =
      await import('../../../rsbuild.config')
    const { configSchema } = await import('@tanstack/router-plugin')
    const options = configSchema.parse({
      target: 'react',
      codeSplittingOptions: {
        compilerPlugins: [routeLazyRecoveryCompilerPlugin],
      },
    })
    expect(options.codeSplittingOptions).toMatchObject({
      compilerPlugins: [routeLazyRecoveryCompilerPlugin],
    })
    const require = createRequire(import.meta.url)
    const compilerPath = path.join(
      path.dirname(require.resolve('@tanstack/router-plugin/package.json')),
      'dist/esm/core/code-splitter/compilers.js'
    )
    const { compileCodeSplitReferenceRoute } = await import(
      /* @vite-ignore */ pathToFileURL(compilerPath).href
    )
    const result = compileCodeSplitReferenceRoute({
      code: `
        import { createFileRoute } from '@tanstack/react-router'
        export const Route = createFileRoute('/probe')({
          component: Screen,
          pendingComponent: Pending,
          loader: async () => ({ ready: true }),
        })
        function Screen() { return <p>Ready</p> }
        function Pending() { return <p>Pending</p> }
      `,
      codeSplitGroupings: [['component'], ['pendingComponent']],
      targetFramework: 'react',
      filename: '/tmp/routes/bootstrap-probe.tsx',
      id: '/tmp/routes/bootstrap-probe.tsx',
      addHmr: false,
      compilerPlugins: [routeLazyRecoveryCompilerPlugin],
    })

    expect(
      result.code.match(/from ['"]@\/lib\/route-lazy-component['"]/g) ?? []
    ).toHaveLength(1)
    expect(result.code).toContain(
      "import('/tmp/routes/bootstrap-probe.tsx?tsr-split=component')"
    )
    expect(result.code).toContain(
      "import('/tmp/routes/bootstrap-probe.tsx?tsr-split=pendingComponent')"
    )
    expect(result.code).toContain("'component')")
    expect(result.code).toContain("'pendingComponent')")
    expect(result.code).toContain("from '@tanstack/react-router'")
    expect(result.code).toContain('loader: async () =>')
  } finally {
    vi.stubGlobal('window', browserWindow)
    vi.stubGlobal('document', browserDocument)
  }
})
