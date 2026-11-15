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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { PricingTable } from '../components/pricing-table'
import {
  PricingToolbar,
  type PricingToolbarProps,
} from '../components/pricing-toolbar'
import type { PricingModel } from '../types'

function toolbarProps(): PricingToolbarProps {
  return {
    filteredCount: 1,
    totalCount: 1,
    sortBy: 'name',
    tokenUnit: 'M',
    showRechargePrice: false,
    viewMode: 'table',
    quotaTypeFilter: 'all',
    endpointTypeFilter: 'all',
    vendorFilter: 'all',
    groupFilter: 'all',
    tagFilter: 'all',
    onSortChange: vi.fn(),
    onTokenUnitChange: vi.fn(),
    onRechargePriceChange: vi.fn(),
    onViewModeChange: vi.fn(),
    onQuotaTypeChange: vi.fn(),
    onEndpointTypeChange: vi.fn(),
    onVendorChange: vi.fn(),
    onGroupChange: vi.fn(),
    onTagChange: vi.fn(),
    vendors: [],
    groups: ['default', 'premium'],
    groupRatios: { default: 1, premium: 3 },
    tags: [],
    models: [],
    hasActiveFilters: false,
    activeFilterCount: 0,
    onClearFilters: vi.fn(),
  }
}

const model: PricingModel = {
  id: 1,
  model_name: 'gpt-4o-mini',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 4,
  enable_groups: ['default'],
}

describe('pricing page layout', () => {
  it('keeps the search box and the group picker on the same 36px filter row', async () => {
    const props = toolbarProps()
    const user = userEvent.setup()
    render(
      <PricingToolbar
        {...props}
        searchValue=''
        onSearchChange={vi.fn()}
        onSearchClear={vi.fn()}
      />
    )

    expect(screen.getByRole('textbox', { name: 'Search models' })).toHaveClass(
      'h-9'
    )
    const groupPicker = screen.getByRole('button', { name: /Group/ })
    expect(groupPicker).toHaveClass('h-9')

    await user.click(groupPicker)
    await user.click(screen.getByRole('menuitem', { name: /premium/i }))
    expect(props.onGroupChange).toHaveBeenCalledWith('premium')
  })

  it('picks the group from the filter bar without opening the sheet', () => {
    render(<PricingToolbar {...toolbarProps()} groupFilter='premium' />)

    const groupPicker = screen.getByRole('button', { name: /Group/ })
    expect(groupPicker).toHaveTextContent('premium')
    expect(groupPicker).toHaveTextContent('Group')
  })

  it('gives model rows a roomier 56px height than the console table', () => {
    render(<PricingTable models={[model]} />)

    const bodyRow = screen.getAllByRole('row').at(-1)
    expect(bodyRow).toHaveClass('[&>td]:h-14')
  })
})
