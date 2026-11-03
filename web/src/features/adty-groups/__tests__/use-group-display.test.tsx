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
import { render, renderHook, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { toContentLang } from '@/lib/adty-lang'
import { useGroupDisplayStore } from '@/stores/adty-group-display-store'

import { GroupDisplaySync } from '../components/group-display-sync'
import { useGroupDisplay } from '../hooks/use-group-display'

const api = vi.hoisted(() => ({ getGroupDisplay: vi.fn() }))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

afterEach(async () => {
  useGroupDisplayStore.setState({ groups: [] })
  await i18next.changeLanguage('en')
})

describe('useGroupDisplay', () => {
  it('uses the current language, then English, then the ID', async () => {
    api.getGroupDisplay.mockResolvedValue([
      {
        id: 'gab12',
        names: { en: 'Pro', 'zh-CN': '专业版' },
        descs: { en: 'Fast lane' },
        usable: true,
        visible: true,
      },
      { id: 'gnone', names: {}, descs: {}, usable: false, visible: true },
    ])
    await i18next.changeLanguage('zhCN')
    render(<GroupDisplaySync />, { wrapper })
    const { result } = renderHook(() => useGroupDisplay())
    await waitFor(() => expect(result.current.groups).toHaveLength(2))

    expect(result.current.name('gab12')).toBe('专业版')
    expect(result.current.desc('gab12')).toBe('Fast lane')
    expect(result.current.name('gnone')).toBe('gnone')
    expect(result.current.name('unknown')).toBe('unknown')
    expect(result.current.adminLabel('gab12')).toBe('专业版 · gab12')
    expect(result.current.adminLabel('gnone')).toBe('gnone')
  })

  it('shows the ID when names are not loaded', () => {
    const { result } = renderHook(() => useGroupDisplay())
    expect(result.current.name('vip')).toBe('vip')
    expect(result.current.adminLabel('vip')).toBe('vip')
  })

  it('falls back to English for a language without a name', async () => {
    api.getGroupDisplay.mockResolvedValue([
      {
        id: 'gab12',
        names: { en: 'Pro' },
        descs: {},
        usable: true,
        visible: true,
      },
    ])
    await i18next.changeLanguage('ja')
    render(<GroupDisplaySync />, { wrapper })
    const { result } = renderHook(() => useGroupDisplay())
    await waitFor(() => expect(result.current.name('gab12')).toBe('Pro'))
  })
})

describe('toContentLang', () => {
  it.each([
    ['zhCN', 'zh-CN'],
    ['zhTW', 'zh-TW'],
    ['en', 'en'],
    ['fr', 'fr'],
    ['ja', 'ja'],
    ['ru', 'ru'],
    ['vi', 'vi'],
    ['xx', 'en'],
  ])('maps %s to %s', (input, expected) => {
    expect(toContentLang(input)).toBe(expected)
  })
})
