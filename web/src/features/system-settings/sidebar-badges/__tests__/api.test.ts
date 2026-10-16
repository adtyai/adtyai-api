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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { getSidebarBadges } from '../api'
import { EMPTY_SIDEBAR_BADGES } from '../types'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
}))

describe('getSidebarBadges', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
  })

  it('returns counts on success', async () => {
    vi.mocked(api.get).mockResolvedValue({
      status: 200,
      data: {
        success: true,
        data: { risk_pending: 2, withdraw_pending: 1, topup_pending: 4 },
      },
    })
    await expect(getSidebarBadges()).resolves.toEqual({
      risk_pending: 2,
      withdraw_pending: 1,
      topup_pending: 4,
      channel_alert: 0,
      reconcile_alert: 0,
    })
  })

  it('returns zeros on 404 / network failure', async () => {
    vi.mocked(api.get).mockResolvedValue({ status: 404, data: {} })
    await expect(getSidebarBadges()).resolves.toEqual(EMPTY_SIDEBAR_BADGES)
    vi.mocked(api.get).mockRejectedValue(new Error('network'))
    await expect(getSidebarBadges()).resolves.toEqual(EMPTY_SIDEBAR_BADGES)
  })
})
