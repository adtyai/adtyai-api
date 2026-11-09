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
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { MODELS_SECTION_IDS } from '@/features/models/section-registry'
import {
  parseSidebarModulesAdmin,
  SIDEBAR_MODULES_DEFAULT,
} from '@/features/system-settings/maintenance/config'
import { MODELS_SECTION_IDS as MODEL_SETTING_SECTION_IDS } from '@/features/system-settings/models/section-registry'
import { useSidebarData } from '@/hooks/use-sidebar-data'

// Subscriptions, redemption codes and upstream invite rewards were deleted in
// tasks 1.2-1.5 (docs/adty/deleted-features.md). An upstream merge must not
// bring their pages, menu entries or ?aff= handling back.
const repoRoot = resolve(import.meta.dirname, '../../../../..')
const webSrc = resolve(repoRoot, 'web/src')

function readSource(path: string): string {
  return readFileSync(resolve(webSrc, path), 'utf8')
}

describe('deleted features stay deleted', () => {
  it('retires deployment navigation while retaining model and protocol settings', () => {
    expect(MODELS_SECTION_IDS).toContain('metadata')
    expect(MODELS_SECTION_IDS).toContain('vendors')
    expect(MODELS_SECTION_IDS).not.toContain('deployments')
    expect(MODEL_SETTING_SECTION_IDS).toContain('global')
    expect(MODEL_SETTING_SECTION_IDS).toContain('gemini')
    expect(MODEL_SETTING_SECTION_IDS).not.toContain('model-deployment')
  })
  // Backend paths too: a restored Go file can still build and pass every Go
  // test, so this is the only place that notices it.
  it('keeps every path in the deletion list absent', () => {
    const deletedPaths = readFileSync(
      resolve(repoRoot, 'docs/adty/deleted-paths.txt'),
      'utf8'
    )
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line !== '' && !line.startsWith('#'))

    expect(deletedPaths).toContain('web/src/features/subscriptions/')
    expect(deletedPaths).toContain('model/subscription.go')
    expect(
      deletedPaths.filter((path) => existsSync(resolve(repoRoot, path)))
    ).toEqual([])
  })

  it('keeps the deleted pages out of the generated route tree', () => {
    const routeTree = readSource('routeTree.gen.ts')

    expect(routeTree).not.toContain('/_authenticated/subscriptions/')
    expect(routeTree).not.toContain('/_authenticated/redemption-codes/')
    expect(routeTree).not.toContain('/_authenticated/playground/')
    expect(routeTree).not.toContain('/_authenticated/marketing/')
  })

  it('keeps the deleted modules out of the admin sidebar defaults', () => {
    expect(SIDEBAR_MODULES_DEFAULT.admin).not.toHaveProperty('subscription')
    expect(SIDEBAR_MODULES_DEFAULT.admin).not.toHaveProperty('redemption')
    expect(SIDEBAR_MODULES_DEFAULT.chat).not.toHaveProperty('playground')
    expect(SIDEBAR_MODULES_DEFAULT.admin).not.toHaveProperty('marketing')
  })

  it('drops the retired Playground switch when loading legacy sidebar settings', () => {
    const config = parseSidebarModulesAdmin(
      JSON.stringify({
        chat: { enabled: false, playground: true, chat: false },
      })
    )

    expect(config.chat).toEqual({ enabled: false, chat: false })
  })

  it('drops the retired marketing switch when loading legacy sidebar settings', () => {
    const config = parseSidebarModulesAdmin(
      JSON.stringify({ admin: { enabled: true, marketing: true, users: true } })
    )

    expect(config.admin).not.toHaveProperty('marketing')
    expect(config.admin.users).toBe(true)
  })

  it('keeps the deleted pages out of the sidebar menu', () => {
    // The referral admin badge in the sidebar reads React Query.
    const client = new QueryClient()
    const { result } = renderHook(() => useSidebarData(), {
      wrapper: (props: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client }, props.children),
    })
    const urls = result.current.navGroups
      .flatMap((group) => group.items)
      .flatMap((item) => [
        item.url,
        ...(item.items ?? []).map((sub) => sub.url),
      ])
      .filter((url) => url !== undefined)
      .map(String)

    expect(urls).toContain('/users')
    expect(
      urls.filter(
        (url) =>
          url.includes('/subscriptions') ||
          url.includes('/redemption-codes') ||
          url.includes('/playground') ||
          url.includes('/marketing')
      )
    ).toEqual([])
  })

  it.each([
    'hooks/use-sidebar-config.ts',
    'features/system-settings/maintenance/sidebar-modules-section.tsx',
  ])('keeps the deleted modules out of the %s constants', (path) => {
    const source = readSource(path)

    expect(source).not.toMatch(/subscription/i)
    expect(source).not.toMatch(/redemption/i)
  })

  it.each([
    'routes/__root.tsx',
    'features/auth/sign-up/components/sign-up-form.tsx',
    'features/auth/api.ts',
  ])('does not read or send the invite code in %s', (path) => {
    const source = readSource(path)

    expect(source).not.toMatch(/\baff(_code)?\b/)
    expect(source).not.toMatch(/affiliate/i)
  })

  it('keeps the aff key out of the frontend cache', () => {
    expect(readSource('lib/frontend-cache.ts')).not.toContain("'aff'")
  })
})
