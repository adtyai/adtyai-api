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

import type { EpayInstanceView } from '../../types'
import {
  formatDailyLimit,
  formatSecretFingerprint,
  primaryInstance,
} from '../display'

function row(
  partial: Partial<EpayInstanceView> & Pick<EpayInstanceView, 'id'>
): EpayInstanceView {
  return {
    name: 'n',
    partner_id: 'p',
    pay_address: 'https://x',
    callback_base: '',
    daily_limit: 0,
    status: 'enabled',
    is_primary: false,
    failure_streak: 0,
    secret_set: true,
    secret_fingerprint: 'hm_abc',
    created_at: '',
    updated_at: '',
    ...partial,
  }
}

describe('epay display helpers', () => {
  it('formats fingerprint and unlimited daily limit', () => {
    expect(formatSecretFingerprint('hm_b81c')).toBe('hm_b81c')
    expect(formatSecretFingerprint('')).toBe('-')
    expect(formatDailyLimit(0)).toBe('unlimited')
    expect(formatDailyLimit(80000)).toBe('80000')
  })

  it('finds enabled primary', () => {
    const rows = [
      row({ id: 1, is_primary: true, status: 'disabled' }),
      row({ id: 2, is_primary: true, status: 'enabled' }),
    ]
    expect(primaryInstance(rows)?.id).toBe(2)
  })
})
