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
import { describe, expect, it } from 'vitest'

import { deploymentPriceSummary } from '@/features/models/lib/deployment-price'
import { DeploymentPriceSummary } from '@/features/models/lib/deployment-price-summary'

describe('deploymentPriceSummary', () => {
  it('reads the nested total the create API returns', () => {
    const data = {
      estimated_cost: 12.5,
      currency: 'USDC',
      price_breakdown: { total_cost: 12.5, hourly_rate: 12.5 },
    }
    expect(deploymentPriceSummary(data)).toBe('12.5 USDC')
    render(<DeploymentPriceSummary data={data} loading={false} />)
    expect(screen.getByText('12.5 USDC')).toBeTruthy()
  })
})
