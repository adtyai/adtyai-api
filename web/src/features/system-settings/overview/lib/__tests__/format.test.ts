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
  formatOverviewAsOf,
  formatOverviewCount,
  formatOverviewDateShort,
  formatOverviewQuota,
  formatOverviewUsd,
} from '../format'

describe('formatOverviewUsd', () => {
  it('formats two decimal places with dollar sign and thousands separators', () => {
    expect(formatOverviewUsd(12.5)).toBe('$12.50')
    expect(formatOverviewUsd(0)).toBe('$0.00')
    expect(formatOverviewUsd(48620)).toBe('$48,620.00')
    expect(formatOverviewUsd(12345.67)).toBe('$12,345.67')
  })

  it('returns em dash for nullish or NaN', () => {
    expect(formatOverviewUsd(null)).toBe('—')
    expect(formatOverviewUsd(undefined)).toBe('—')
    expect(formatOverviewUsd(Number.NaN)).toBe('—')
  })
})

describe('formatOverviewCount', () => {
  it('truncates to integer string with thousands separators', () => {
    expect(formatOverviewCount(412)).toBe('412')
    expect(formatOverviewCount(3.9)).toBe('3')
    expect(formatOverviewCount(0)).toBe('0')
    expect(formatOverviewCount(1234)).toBe('1,234')
  })

  it('returns em dash for nullish or NaN', () => {
    expect(formatOverviewCount(null)).toBe('—')
    expect(formatOverviewCount(undefined)).toBe('—')
    expect(formatOverviewCount(Number.NaN)).toBe('—')
  })
})

describe('formatOverviewQuota', () => {
  it('returns em dash for nullish or NaN', () => {
    expect(formatOverviewQuota(null)).toBe('—')
    expect(formatOverviewQuota(undefined)).toBe('—')
    expect(formatOverviewQuota(Number.NaN)).toBe('—')
  })

  it('formats finite quota (not em dash)', () => {
    expect(formatOverviewQuota(0)).not.toBe('—')
    expect(formatOverviewQuota(500000)).not.toBe('—')
  })
})

describe('formatOverviewDateShort', () => {
  it('formats YYYY-MM-DD as M/D', () => {
    expect(formatOverviewDateShort('2026-10-15')).toBe('10/15')
    expect(formatOverviewDateShort('2026-01-03')).toBe('1/3')
  })

  it('falls back for invalid input', () => {
    expect(formatOverviewDateShort('')).toBe('—')
    expect(formatOverviewDateShort(null)).toBe('—')
    expect(formatOverviewDateShort('not-a-date')).toBe('not-a-date')
  })
})

describe('formatOverviewAsOf', () => {
  it('formats RFC3339 in Asia/Shanghai', () => {
    // 2026-10-15T08:40:00Z = 16:40 Asia/Shanghai
    const out = formatOverviewAsOf('2026-10-15T08:40:00Z', 'Asia/Shanghai')
    expect(out).toMatch(/2026/)
    expect(out).toMatch(/16:40:00/)
  })

  it('returns em dash or raw on bad input', () => {
    expect(formatOverviewAsOf(null)).toBe('—')
    expect(formatOverviewAsOf('')).toBe('—')
    expect(formatOverviewAsOf('not-rfc3339')).toBe('not-rfc3339')
  })
})
