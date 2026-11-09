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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { PaymentSceneConfig } from '../api'
import { PaymentSceneManager } from '../index'

const originalAdapter = api.defaults.adapter
let client: QueryClient
let config: PaymentSceneConfig
let writes: { expected_version: number; scenes: PaymentSceneConfig['scenes'] }[]
let conflict: boolean

beforeEach(() => {
  writes = []
  conflict = false
  config = {
    version: 3,
    scenes: [
      {
        key: 'wallet_topup',
        name: 'Wallet top-up',
        business: 'wallet_topup',
        enabled: true,
        providers: ['stripe'],
        methods: [],
        channel_ids: [],
        currencies: ['USD'],
        site_scope: 'all',
        site_ids: [],
        user_types: [],
        min_usd: '0',
        max_usd: '0',
      },
    ],
    provider_options: [{ provider: 'stripe', label: 'Stripe' }],
    method_options: [
      { id: 'stripe', label: 'Stripe', currency: 'USD', available: true },
    ],
  }
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  api.defaults.adapter = (async (request) => {
    if (request.url !== '/api/adty/admin/payment-scenes') {
      throw new Error(`Unexpected request ${request.url}`)
    }
    if (request.method === 'put') {
      const body = JSON.parse(request.data)
      writes.push(body)
      if (!conflict) {
        config = { ...config, version: config.version + 1, scenes: body.scenes }
      }
    }
    return {
      config: request,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: {
        success: !conflict,
        message: conflict
          ? 'Payment scene settings changed. Refresh and try again.'
          : '',
        data: config,
      },
    }
  }) satisfies AxiosAdapter
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
})

function renderManager() {
  render(
    <QueryClientProvider client={client}>
      <PaymentSceneManager />
    </QueryClientProvider>
  )
}

it('saves only after confirmation and sends the version captured when editing', async () => {
  const user = userEvent.setup()
  renderManager()
  await user.click(
    await screen.findByRole('button', {
      name: 'Edit payment scene Wallet top-up',
    })
  )
  const name = screen.getByRole('textbox', { name: 'Name' })
  await user.clear(name)
  await user.type(name, 'Edited checkout')
  await user.click(screen.getByRole('button', { name: 'Save payment scene' }))
  expect(writes).toHaveLength(0)
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Cancel',
    })
  )
  expect(writes).toHaveLength(0)
  expect(name).toHaveValue('Edited checkout')
  await user.click(screen.getByRole('button', { name: 'Save payment scene' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(writes).toHaveLength(1)
  expect(writes[0].expected_version).toBe(3)
  expect(writes[0].scenes[0].name).toBe('Edited checkout')
  expect(await screen.findByText('Edited checkout')).toBeInTheDocument()
})

it('retains an edited draft after a version conflict', async () => {
  const user = userEvent.setup()
  renderManager()
  await user.click(
    await screen.findByRole('button', {
      name: 'Edit payment scene Wallet top-up',
    })
  )
  const name = screen.getByRole('textbox', { name: 'Name' })
  await user.clear(name)
  await user.type(name, 'Unsaved checkout')
  conflict = true
  await user.click(screen.getByRole('button', { name: 'Save payment scene' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Payment scene settings changed'
  )
  expect(name).toHaveValue('Unsaved checkout')
  expect(config.scenes[0].name).toBe('Wallet top-up')
})

it('deletes only after confirmation', async () => {
  const user = userEvent.setup()
  renderManager()
  await user.click(
    await screen.findByRole('button', {
      name: 'Delete payment scene Wallet top-up',
    })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Cancel',
    })
  )
  expect(writes).toHaveLength(0)
  await user.click(
    screen.getByRole('button', { name: 'Delete payment scene Wallet top-up' })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  expect(await screen.findByText('No payment scenes')).toBeInTheDocument()
  expect(writes[0]).toEqual({ expected_version: 3, scenes: [] })
})

it('hides scene management from ordinary administrators', () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
  renderManager()
  expect(screen.queryByText('Payment scenes')).not.toBeInTheDocument()
  expect(writes).toHaveLength(0)
})

it('names scene restrictions and selects a real provider when creating a scene', async () => {
  const user = userEvent.setup()
  renderManager()
  await user.click(
    await screen.findByRole('button', { name: 'New payment scene' })
  )
  const dialog = within(screen.getByRole('dialog'))
  for (const name of [
    'Payment providers',
    'Payment methods',
    'Currencies',
    'User types',
  ]) {
    expect(dialog.getByRole('combobox', { name })).toBeInTheDocument()
  }
  await user.click(dialog.getByRole('combobox', { name: 'Payment providers' }))
  await user.click(await screen.findByRole('option', { name: 'Stripe' }))
  await user.keyboard('{Escape}')
  await user.type(
    dialog.getByRole('textbox', { name: 'Scene identifier' }),
    'checkout'
  )
  await user.type(dialog.getByRole('textbox', { name: 'Name' }), 'New checkout')
  await user.click(dialog.getByRole('button', { name: 'Save payment scene' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Continue',
    })
  )
  await waitFor(() => expect(writes).toHaveLength(1))
  expect(writes[0].scenes[1]).toMatchObject({
    key: 'checkout',
    name: 'New checkout',
    providers: ['stripe'],
  })
})
