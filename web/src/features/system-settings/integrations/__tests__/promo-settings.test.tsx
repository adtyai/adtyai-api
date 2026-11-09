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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { AdtyPromoSettingsSection } from '../adty-promo-settings-section'

function Fixture() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <AdtyPromoSettingsSection defaultEnabled />
    </SettingsPageProvider>
  )
}

function renderSettings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Fixture />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})
afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

test('saving the inherited flag persists its original key and clears the draft only after success', async () => {
  const user = userEvent.setup()
  renderSettings()
  const toggle = screen.getByRole('switch', {
    name: 'Allow users to enter promo codes',
  })
  expect(toggle).toBeChecked()
  const save = screen.getByRole('button', { name: 'Save Changes' })
  expect(save).toBeDisabled()
  await user.click(toggle)
  expect(save).toBeEnabled()
  await user.click(save)
  await waitFor(() => expect(save).toBeDisabled())
  expect(api.put).toHaveBeenCalledWith('/api/option/', {
    key: 'adty_marketing_setting.promo_enabled',
    value: 'false',
  })
  expect(toggle).not.toBeChecked()
})

test('losing Root while editing prevents saving the promo flag', async () => {
  const user = userEvent.setup()
  renderSettings()
  await user.click(
    screen.getByRole('switch', { name: 'Allow users to enter promo codes' })
  )
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 10 })
  )
  expect(
    screen.getByRole('switch', { name: 'Allow users to enter promo codes' })
  ).toHaveAttribute('aria-disabled', 'true')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(api.put).not.toHaveBeenCalled()
})

test('a rejected flag save retains the draft and permits a successful retry', async () => {
  vi.mocked(api.put).mockResolvedValueOnce({
    data: { success: false, message: 'Save denied' },
  })
  const user = userEvent.setup()
  renderSettings()
  const toggle = screen.getByRole('switch', {
    name: 'Allow users to enter promo codes',
  })
  await user.click(toggle)
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  const save = await screen.findByRole('button', { name: 'Save Changes' })
  await waitFor(() => expect(save).toBeEnabled())
  expect(toggle).not.toBeChecked()
  await user.click(save)
  await waitFor(() => expect(save).toBeDisabled())
})
