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
import { describe, expect, it } from 'vitest'

import {
  CHART_PALETTE_FALLBACK,
  chartPaletteRangeFaded,
} from '@/lib/chart-theme'

const LIGHT = [...CHART_PALETTE_FALLBACK.light]

describe('chartPaletteRangeFaded', () => {
  it('keeps the first lap identical to the palette', () => {
    expect(chartPaletteRangeFaded(5, LIGHT)).toEqual(LIGHT)
  })

  it('fades every repeat so series six is not a clone of series one', () => {
    const colours = chartPaletteRangeFaded(7, LIGHT)
    expect(colours.slice(0, 5)).toEqual(LIGHT)
    expect(colours[5]).toBe('rgba(0, 112, 236, 0.8)')
    expect(colours[6]).toBe('rgba(79, 163, 255, 0.8)')
    expect(colours[5]).not.toBe(colours[0])
  })

  it('stops fading at the floor and leaves non-hex entries alone', () => {
    const colours = chartPaletteRangeFaded(16, LIGHT)
    expect(colours[15]).toBe('rgba(0, 112, 236, 0.4)')
    expect(
      chartPaletteRangeFaded(2, ['oklch(0.5 0.1 250)', '#0070ec'])[1]
    ).toBe('#0070ec')
  })

  it('returns nothing for an empty request', () => {
    expect(chartPaletteRangeFaded(0, LIGHT)).toEqual([])
    expect(chartPaletteRangeFaded(3, [])).toEqual([])
  })
})
