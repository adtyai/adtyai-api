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
import { render, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DOC_PAGES, getDocMarkdown } from '../content'
import { DocPage } from '../doc-page'

vi.mock('@/features/adty-site/api', () => ({
  useApiBaseUrl: () => 'https://api.example.com',
}))
vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children: ReactNode }) => <div>{props.children}</div>,
}))

function renderDoc(slug: string) {
  const rootRoute = createRootRoute()
  const docRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/docs/$slug',
    component: () => <DocPage slug={slug} />,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([docRoute]),
    history: createMemoryHistory({ initialEntries: [`/docs/${slug}`] }),
  })
  render(<RouterProvider router={router} />)
}

afterEach(async () => {
  await i18next.changeLanguage('en')
})

describe('DocPage', () => {
  it('shows the Chinese page when the interface language is zhCN', async () => {
    await i18next.changeLanguage('zhCN')
    renderDoc('quickstart')
    expect(
      await screen.findByRole('heading', { level: 1, name: '快速开始' })
    ).toBeVisible()
  })

  // Task 9.2 added every page in all eight languages (docs-complete.test.ts),
  // so the English fallback in getDocMarkdown is no longer reached.
  it('shows the Japanese page when the interface language is ja', async () => {
    await i18next.changeLanguage('ja')
    renderDoc('quickstart')
    expect(
      await screen.findByRole('heading', { level: 1, name: 'クイックスタート' })
    ).toBeVisible()
  })

  it('replaces the base URL placeholder with the site API address', async () => {
    renderDoc('claude-code')
    await screen.findByRole('heading', { level: 1, name: 'Claude Code' })
    expect(document.body.textContent).toContain(
      'export ANTHROPIC_BASE_URL="https://api.example.com"'
    )
    expect(document.body.textContent).not.toContain('{{BASE_URL}}')
  })

  it('marks the open page in the doc navigation', async () => {
    renderDoc('codex-cli')
    const nav = await screen.findByRole('navigation', {
      name: 'Documentation',
    })
    const current = nav.querySelector('[aria-current="page"]')
    expect(current).toHaveTextContent('Codex CLI')
    expect(nav.querySelectorAll('a')).toHaveLength(DOC_PAGES.length)
  })

  it('shows not found with a link to the quick start for an unknown slug', async () => {
    renderDoc('nope')
    expect(await screen.findByText('Page not found')).toBeVisible()
    const article = screen.getByRole('article')
    expect(
      within(article).getByText('Quick start').closest('a')
    ).toHaveAttribute('href', '/docs/quickstart')
  })
})

describe('doc content', () => {
  it.each(DOC_PAGES)('has distinct English and Chinese text for %s', (slug) => {
    const en = getDocMarkdown(slug, 'en')
    const zh = getDocMarkdown(slug, 'zh-CN')
    expect(en).toMatch(/^# /)
    expect(zh).toMatch(/^# /)
    expect(zh).not.toBe(en)
  })
})
