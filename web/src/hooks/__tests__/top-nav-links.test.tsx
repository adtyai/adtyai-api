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
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { useTopNavLinks } from '../use-top-nav-links'

const clients: QueryClient[] = []
const businessIds = [
  'home',
  'console',
  'pricing',
  'rankings',
  'docs',
  'about',
  'affiliate',
]

beforeEach(() => {
  useAuthStore.getState().auth.reset()
  i18next.addResourceBundle('fr', 'translation', {
    Home: 'Accueil',
    Docs: 'Documentation',
  })
})

afterEach(async () => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset()
  await i18next.changeLanguage('en')
  i18next.removeResourceBundle('fr', 'translation')
  localStorage.clear()
})

function renderLinks(status: Record<string, unknown> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['status'], status)
  const result = renderHook(() => useTopNavLinks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    ),
  })
  return { ...result, client }
}

describe('top navigation business identity', () => {
  it('assigns a unique fixed business ID to every default navigation item', () => {
    const { result } = renderLinks()

    expect(result.current).toEqual([
      { id: 'home', title: 'Home', href: '/' },
      { id: 'console', title: 'Console', href: '/dashboard' },
      {
        id: 'pricing',
        title: 'Model Square',
        href: '/pricing',
        requiresAuth: false,
      },
      {
        id: 'rankings',
        title: 'Rankings',
        href: '/rankings',
        requiresAuth: false,
      },
      { id: 'docs', title: 'Docs', href: '/docs' },
      { id: 'about', title: 'About', href: '/about' },
      { id: 'affiliate', title: 'Affiliate Program', href: '/affiliate' },
    ])
    expect(new Set(result.current.map((link) => link.id)).size).toBe(7)
  })

  it('keeps business IDs while translated navigation titles change', async () => {
    const { result } = renderLinks()

    await act(async () => {
      await i18next.changeLanguage('fr')
    })

    expect(result.current.map((link) => link.id)).toEqual(businessIds)
    expect(result.current.find((link) => link.id === 'home')?.title).toBe(
      'Accueil'
    )
    expect(result.current.find((link) => link.id === 'docs')?.title).toBe(
      'Documentation'
    )
  })

  it('keeps Docs identity when its configured destination changes', async () => {
    const { result, client } = renderLinks()

    await act(async () => {
      client.setQueryData(['status'], {
        docs_link: 'https://docs.example.com/guide',
      })
    })

    await waitFor(() =>
      expect(result.current.find((link) => link.id === 'docs')).toEqual({
        id: 'docs',
        title: 'Docs',
        href: 'https://docs.example.com/guide',
        external: true,
      })
    )
    expect(result.current.map((link) => link.id)).toEqual(businessIds)
  })

  it('preserves enabled modules and auth gating while hidden modules return', async () => {
    const { result, client } = renderLinks({
      HeaderNavModules: {
        home: false,
        docs: false,
        affiliate: false,
        pricing: { enabled: true, requireAuth: true },
        rankings: { enabled: false, requireAuth: true },
      },
    })
    expect(result.current.map((link) => link.id)).toEqual([
      'console',
      'pricing',
      'about',
    ])
    expect(
      result.current.find((link) => link.id === 'pricing')?.requiresAuth
    ).toBe(true)

    await act(async () => {
      useAuthStore
        .getState()
        .auth.setUser({ id: 1, username: 'member', role: 1 })
      client.setQueryData(['status'], {
        HeaderNavModules: JSON.stringify({
          pricing: { enabled: true, requireAuth: true },
          rankings: { enabled: true, requireAuth: true },
        }),
      })
    })

    await waitFor(() =>
      expect(result.current.map((link) => link.id)).toEqual(businessIds)
    )
    expect(
      result.current.find((link) => link.id === 'pricing')?.requiresAuth
    ).toBe(false)
    expect(
      result.current.find((link) => link.id === 'rankings')?.requiresAuth
    ).toBe(false)
  })
})
