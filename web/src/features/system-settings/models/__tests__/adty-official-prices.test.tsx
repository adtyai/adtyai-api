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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { AdtyOfficialPricesSection } from '../adty-official-prices-section'

const rows = [
  {
    model_name: 'claude-sonnet-4-5',
    source_id: 'anthropic/claude-sonnet-4-5',
    input_per_mtok: '3',
    output_per_mtok: '15',
    cache_read_per_mtok: '0.3',
    cache_write_per_mtok: '3.75',
    per_call: null,
    source: 'models.dev',
    override: false,
    synced_at: 1767225600,
    updated_at: 1767225600,
  },
  {
    model_name: 'gpt-4o',
    source_id: 'openai/gpt-4o',
    input_per_mtok: '2.5',
    output_per_mtok: '10',
    cache_read_per_mtok: null,
    cache_write_per_mtok: null,
    per_call: null,
    source: 'models.dev',
    override: false,
    synced_at: 1767225600,
    updated_at: 1767225600,
  },
  {
    model_name: 'deepseek/deepseek-chat',
    source_id: '',
    input_per_mtok: null,
    output_per_mtok: null,
    cache_read_per_mtok: null,
    cache_write_per_mtok: null,
    per_call: null,
    source: '',
    override: false,
    synced_at: 0,
    updated_at: 0,
  },
]

function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AdtyOfficialPricesSection />
    </QueryClientProvider>
  )
}

function modelNames() {
  return screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('cell')[0].textContent)
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: rows },
  })
})
afterEach(() => {
  vi.restoreAllMocks()
})

describe('AdtyOfficialPricesSection', () => {
  it('filters rows by provider and by model ID', async () => {
    const user = userEvent.setup()
    renderSection()
    await screen.findByText('claude-sonnet-4-5')
    expect(api.get).toHaveBeenCalledWith('/api/adty/admin/official-prices')

    await user.selectOptions(screen.getByLabelText('Provider'), 'openai')
    expect(modelNames()).toEqual(['gpt-4o'])

    await user.selectOptions(screen.getByLabelText('Provider'), '__unmapped')
    expect(modelNames()).toEqual(['deepseek/deepseek-chat'])

    await user.selectOptions(screen.getByLabelText('Provider'), '')
    await user.type(screen.getByLabelText('Search model ID'), 'sonnet')
    expect(modelNames()).toEqual(['claude-sonnet-4-5'])
  })

  it('saves an edited mapping and prices for a model name with a slash', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, data: {} },
    })
    const user = userEvent.setup()
    renderSection()
    const row = (await screen.findByText('deepseek/deepseek-chat')).closest(
      'tr'
    )
    if (!row) throw new Error('missing row')
    await user.click(within(row).getByRole('button', { name: 'Edit' }))

    const dialog = await screen.findByRole('dialog')
    await user.type(
      within(dialog).getByLabelText('models.dev ID'),
      'deepseek/deepseek-chat'
    )
    await user.type(within(dialog).getByLabelText('Input'), '0.27')
    await user.type(within(dialog).getByLabelText('Output'), '1.1')
    await user.click(within(dialog).getByRole('switch'))
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))

    expect(put).toHaveBeenCalledWith(
      '/api/adty/admin/official-prices/deepseek/deepseek-chat',
      {
        source_id: 'deepseek/deepseek-chat',
        input_per_mtok: '0.27',
        output_per_mtok: '1.1',
        cache_read_per_mtok: null,
        cache_write_per_mtok: null,
        per_call: null,
        override: true,
      }
    )
  })

  it('blocks saving a models.dev ID without a provider prefix', async () => {
    const put = vi.spyOn(api, 'put')
    const user = userEvent.setup()
    renderSection()
    const row = (await screen.findByText('gpt-4o')).closest('tr')
    if (!row) throw new Error('missing row')
    await user.click(within(row).getByRole('button', { name: 'Edit' }))
    const dialog = await screen.findByRole('dialog')
    const input = within(dialog).getByLabelText('models.dev ID')
    await user.clear(input)
    await user.type(input, 'gpt-4o')
    expect(within(dialog).getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(put).not.toHaveBeenCalled()
  })

  it('runs a sync when the sync button is clicked', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { updated: 2, synced_at: 1767225600 } },
    })
    const user = userEvent.setup()
    renderSection()
    await screen.findByText('claude-sonnet-4-5')
    await user.click(screen.getByRole('button', { name: 'Sync now' }))
    expect(post).toHaveBeenCalledWith('/api/adty/admin/official-prices/sync')
  })
})
