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
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { ThemeSwitch } from '@/components/theme-switch'
import { ThemeProvider } from '@/context/theme-provider'
import { useSystemConfig } from '@/hooks/use-system-config'
import { api } from '@/lib/api'
import { STATUS_QUERY_KEY, mapStatusDataToConfig } from '@/lib/status-query'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { OwnerVerificationProvider } from '../components/owner-verification'
import { SiteSettingsPanel } from '../components/site-settings-panel'
import type { OwnerSite } from '../types'

const site: OwnerSite = {
  id: 7,
  status: 'active',
  subdomain: 'shop',
  root_domain: 'sites.example.test',
  host: 'shop.sites.example.test',
  base_url: 'https://shop.sites.example.test',
  cost_factor: '0.8',
  markup_min: '1',
  markup_max: '3',
  markup_increase_delay_hours: 24,
  custom_domain_enabled: false,
  sender_domain_enabled: false,
  brand: {
    name: 'Shop AI',
    logo_url: '/uploads/site-assets/logo.png',
    tagline: '',
    notice: '',
    support_contact: '',
  },
  bonus_rates: {},
  seo_index: false,
  agreement_version: '1',
  agreement_at: 1,
  prices: [],
  pricing_version: 1,
  domains: [],
  visibility: {},
}
let client: QueryClient
const initialTitle = document.title
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  document.head.insertAdjacentHTML(
    'beforeend',
    '<meta name="theme-color" content="#fff"><link rel="icon" href="/favicon.ico">'
  )
})
afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  document
    .querySelectorAll('meta[name="theme-color"], link[rel~="icon"]')
    .forEach((element) => element.remove())
  document.documentElement.removeAttribute('style')
  document.documentElement.classList.remove('light', 'dark')
  document.title = initialTitle
})

it('saves an independent favicon, website title and theme through the real brand API and invalidates public status', async () => {
  const user = userEvent.setup()
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: site } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { url: '/uploads/site-assets/icon.webp' } },
  })
  client.setQueryData(STATUS_QUERY_KEY, { system_name: 'Shop AI' })
  render(
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>
        <SiteSettingsPanel site={site} />
      </OwnerVerificationProvider>
    </QueryClientProvider>
  )
  await user.type(screen.getByLabelText('Website title'), 'Shop Workspace')
  await user.type(screen.getByLabelText('Theme color'), '#225588')
  await user.upload(
    screen.getByLabelText('Favicon'),
    new File(['synthetic-image'], 'icon.webp', { type: 'image/webp' })
  )
  await waitFor(() =>
    expect(screen.getByAltText('Favicon')).toHaveAttribute(
      'src',
      '/uploads/site-assets/icon.webp'
    )
  )
  await user.click(screen.getByRole('button', { name: 'Save brand' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/adty/site/self', {
      brand: expect.objectContaining({
        title: 'Shop Workspace',
        theme_color: '#225588',
        favicon_url: '/uploads/site-assets/icon.webp',
        logo_url: '/uploads/site-assets/logo.png',
      }),
    })
  )
  expect(post).toHaveBeenCalledWith('/api/adty/site/logo', expect.any(FormData))
  await waitFor(() =>
    expect(client.getQueryState(STATUS_QUERY_KEY)?.isInvalidated).toBe(true)
  )
})

function BrandingConsumer() {
  useSystemConfig()
  return <ThemeSwitch />
}

it('applies site branding independently from the logo and keeps the site color across light and dark themes, then clears stale overrides', async () => {
  const user = userEvent.setup()
  useSystemConfigStore.getState().setConfig(
    mapStatusDataToConfig({
      system_name: 'Shop AI',
      site_title: 'Shop Workspace',
      logo: '/uploads/site-assets/logo.png',
      favicon: '/uploads/site-assets/icon.webp',
      theme_color: '#225588',
    })
  )
  render(
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='light'>
        <BrandingConsumer />
      </ThemeProvider>
    </QueryClientProvider>
  )
  await waitFor(() => expect(document.title).toBe('Shop Workspace'))
  expect(document.querySelector('link[rel~="icon"]')).toHaveAttribute(
    'href',
    '/uploads/site-assets/icon.webp'
  )
  expect(document.documentElement.style.getPropertyValue('--primary')).toBe(
    '#225588'
  )
  expect(
    document.documentElement.style.getPropertyValue('--primary-foreground')
  ).toBe('#ffffff')
  await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
  await user.click(screen.getByRole('menuitem', { name: 'Dark' }))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
    'content',
    '#225588'
  )
  act(() =>
    useSystemConfigStore
      .getState()
      .setConfig(mapStatusDataToConfig({ system_name: 'Main site' }))
  )
  await waitFor(() => expect(document.title).toBe('Main site'))
  expect(document.querySelector('link[rel~="icon"]')).toHaveAttribute(
    'href',
    '/favicon.ico'
  )
  expect(document.documentElement.style.getPropertyValue('--primary')).toBe('')
  expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute(
    'content',
    '#020817'
  )
})
