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
import { renderHook, waitFor } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { PricingData } from '../../types'
import { modelsFromPricing, usePricingData } from '../use-pricing-data'

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { price: 1, usd_exchange_rate: 1 } }),
}))

const pricing = {
  success: true,
  data: [
    {
      id: 1,
      model_name: 'gpt',
      vendor_id: 7,
      quota_type: 0,
      model_ratio: 1,
      completion_ratio: 1,
      enable_groups: ['default'],
    },
  ],
  vendors: [{ id: 7, name: 'OpenAI', icon: 'o', description: 'vendor' }],
  group_ratio: { default: 1 },
  usable_group: {},
  supported_endpoint: {},
  auto_groups: [],
} satisfies PricingData

vi.mock('../../api', () => ({
  getPricing: () => Promise.resolve(pricing),
}))

const client = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client }, children)
}

describe('usePricingData', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the same model list when the price payload is unchanged', async () => {
    const first = renderHook(() => usePricingData(), { wrapper })
    await waitFor(() => expect(first.result.current.models).toHaveLength(1))
    const second = renderHook(() => usePricingData(), { wrapper })
    await waitFor(() => expect(second.result.current.models).toHaveLength(1))

    expect(second.result.current.models).toBe(first.result.current.models)
    expect(modelsFromPricing(pricing)).toBe(first.result.current.models)
    expect(second.result.current.models[0]).not.toBe(pricing.data[0])
    expect(second.result.current.groupRatio).toEqual({ default: 1 })
    expect(second.result.current.models[0].group_ratio).toBe(
      pricing.group_ratio
    )
    expect(second.result.current.models[0].vendor_name).toBe('OpenAI')
  })

  it('does not write display fields onto the pricing response', async () => {
    const before = { ...pricing.data[0] }
    const hook = renderHook(() => usePricingData(), { wrapper })
    await waitFor(() =>
      expect(hook.result.current.models[0]?.vendor_name).toBe('OpenAI')
    )

    expect(pricing.data[0]).toEqual(before)
    expect(hook.result.current.models[0]).not.toBe(pricing.data[0])
    expect(hook.result.current.models[0].group_ratio).toBe(pricing.group_ratio)
  })

  it('returns no models when the pricing payload is not a model list', () => {
    expect(
      modelsFromPricing({
        ...pricing,
        data: { items: [] } as unknown as PricingData['data'],
      })
    ).toEqual([])
  })

  it('rebuilds when the group ratio changes but the model list does not', () => {
    const first = modelsFromPricing(pricing)
    const changed = {
      ...pricing,
      group_ratio: { default: 2 },
    }
    const second = modelsFromPricing(changed)
    expect(second).not.toBe(first)
    expect(second[0].group_ratio).toEqual({ default: 2 })
  })
})
