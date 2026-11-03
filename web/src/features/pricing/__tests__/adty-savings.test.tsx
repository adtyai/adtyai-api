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
import { render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createJSONStorage } from 'zustand/middleware'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { ModelCard } from '../components/model-card'
import { PricingTable } from '../components/pricing-table'
import { getSavingsRatio, type OfficialPrices } from '../lib/adty-savings'
import type { PricingModel } from '../types'

function pricingModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'site-model',
    quota_type: 0,
    // model_ratio 1.5 → $3 per 1M input tokens
    model_ratio: 1.5,
    completion_ratio: 5,
    enable_groups: ['default'],
    group_ratio: { default: 1 },
    ...overrides,
  }
}

const officialPrices: OfficialPrices = {
  'site-model': {
    input: '15',
    output: '75',
    cache_read: null,
    cache_write: null,
    per_call: null,
  },
}

const originalStorage = useSystemConfigStore.persist.getOptions().storage
beforeEach(() => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
    key: (index: number) => [...storage.keys()][index] ?? null,
    get length() {
      return storage.size
    },
  })
  useSystemConfigStore.persist.setOptions({
    storage: createJSONStorage(() => localStorage),
  })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
})
afterEach(() => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  vi.unstubAllGlobals()
  useSystemConfigStore.persist.setOptions({ storage: originalStorage })
})

function cellOf(row: HTMLElement, header: string): HTMLElement {
  const headers = screen.getAllByRole('columnheader')
  const index = headers.findIndex((cell) => cell.textContent === header)
  if (index < 0) throw new Error(`Missing column ${header}`)
  return within(row).getAllByRole('cell')[index]
}

describe('official price savings', () => {
  it('compares the input price including the group ratio for token models', () => {
    expect(getSavingsRatio(pricingModel(), officialPrices['site-model'])).toBe(
      0.8
    )
    expect(
      getSavingsRatio(
        pricingModel({
          enable_groups: ['vip'],
          group_ratio: { vip: 2 },
        }),
        officialPrices['site-model'],
        'vip'
      )
    ).toBeCloseTo(0.6)
  })

  it('compares the per-call price for per-request models', () => {
    expect(
      getSavingsRatio(pricingModel({ quota_type: 1, model_price: 0.02 }), {
        input: null,
        output: null,
        cache_read: null,
        cache_write: null,
        per_call: '0.04',
      })
    ).toBe(0.5)
  })

  it('returns null without a matching or positive official price', () => {
    expect(getSavingsRatio(pricingModel(), undefined)).toBeNull()
    expect(
      getSavingsRatio(pricingModel({ quota_type: 1, model_price: 0.02 }), {
        ...officialPrices['site-model'],
        per_call: null,
      })
    ).toBeNull()
    expect(
      getSavingsRatio(pricingModel(), {
        ...officialPrices['site-model'],
        input: '0',
      })
    ).toBeNull()
  })

  it('shows the official price and an 80% saving when the site charges $3 against $15', () => {
    render(
      <PricingTable models={[pricingModel()]} officialPrices={officialPrices} />
    )
    const row = screen.getByRole('row', { name: /site-model/ })
    expect(cellOf(row, 'Official Price')).toHaveTextContent('$15')
    expect(cellOf(row, 'Official Price')).toHaveTextContent('$75')
    expect(cellOf(row, 'Savings')).toHaveTextContent('80%')
  })

  it('leaves the official price and savings empty for a model without an official price', () => {
    render(
      <PricingTable
        models={[pricingModel({ model_name: 'unlisted-model' })]}
        officialPrices={officialPrices}
      />
    )
    const row = screen.getByRole('row', { name: /unlisted-model/ })
    expect(cellOf(row, 'Official Price')).toHaveTextContent(/^$/)
    expect(cellOf(row, 'Savings')).toHaveTextContent(/^$/)
  })

  it('shows a dash instead of a negative saving when the site price is higher', () => {
    render(
      <PricingTable
        models={[pricingModel({ model_ratio: 10 })]}
        officialPrices={officialPrices}
      />
    )
    const row = screen.getByRole('row', { name: /site-model/ })
    expect(cellOf(row, 'Savings')).toHaveTextContent('—')
  })

  it('shows the saving on the model card when an official price is passed', () => {
    render(
      <ModelCard
        model={pricingModel()}
        onClick={vi.fn()}
        officialPrice={officialPrices['site-model']}
      />
    )
    expect(screen.getByText('Save 80%')).toBeVisible()
  })
})

describe('hidden groups', () => {
  const model = pricingModel({
    enable_groups: ['default', 'internal-vip'],
    group_ratio: { default: 1 },
  })

  it('lists only the groups the viewer can use in the table', () => {
    render(<PricingTable models={[model]} usableGroups={['default']} />)
    const row = screen.getByRole('row', { name: /site-model/ })
    expect(cellOf(row, 'Groups')).toHaveTextContent('default')
    expect(row).not.toHaveTextContent('internal-vip')
  })

  it('lists only the groups the viewer can use on the card', () => {
    render(
      <ModelCard model={model} onClick={vi.fn()} usableGroups={['default']} />
    )
    const groupField = screen.getByText('Groups').parentElement
    expect(groupField).toHaveTextContent('default')
    expect(groupField).not.toHaveTextContent('internal-vip')
    expect(groupField).not.toHaveTextContent('+1')
  })
})
