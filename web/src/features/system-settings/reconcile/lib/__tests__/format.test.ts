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
  defaultReconcileDate,
  formatAccountDriftUsers,
  formatLastBackup,
  formatMissingBuckets,
  hasMissingBuckets,
  reconcileStatusLabelKey,
  reconcileStatusVariant,
  showFailedNote,
  showIncompleteOnly,
  showMismatchDetails,
  showOkSummary,
} from '../format'

describe('defaultReconcileDate', () => {
  it('returns yesterday in Asia/Shanghai', () => {
    // 2026-10-16 01:00 UTC+8 = 2026-10-15 17:00 UTC → yesterday SH = 2026-10-15
    expect(defaultReconcileDate(new Date('2026-10-15T17:00:00.000Z'))).toBe(
      '2026-10-15'
    )
  })

  it('crosses midnight Shanghai correctly', () => {
    // 2026-10-16 00:30 UTC+8 = 2026-10-15 16:30 UTC → yesterday SH = 2026-10-15
    expect(defaultReconcileDate(new Date('2026-10-15T16:30:00.000Z'))).toBe(
      '2026-10-15'
    )
    // 2026-10-15 23:30 UTC+8 = 2026-10-15 15:30 UTC → yesterday SH = 2026-10-14
    expect(defaultReconcileDate(new Date('2026-10-15T15:30:00.000Z'))).toBe(
      '2026-10-14'
    )
  })
})

describe('reconcile status panes', () => {
  it('maps status label keys', () => {
    expect(reconcileStatusLabelKey('ok')).toBe('Reconcile OK')
    expect(reconcileStatusLabelKey('incomplete')).toBe('Reconcile incomplete')
    expect(reconcileStatusLabelKey('mismatch')).toBe('Reconcile mismatch')
    expect(reconcileStatusLabelKey('failed')).toBe('Reconcile failed')
  })

  it('picks badge variants', () => {
    expect(reconcileStatusVariant('ok')).toBe('secondary')
    expect(reconcileStatusVariant('incomplete')).toBe('outline')
    expect(reconcileStatusVariant('mismatch')).toBe('destructive')
    expect(reconcileStatusVariant('failed')).toBe('destructive')
  })

  it('gates UI panes by status', () => {
    expect(showIncompleteOnly('incomplete')).toBe(true)
    expect(showIncompleteOnly('ok')).toBe(false)
    expect(showMismatchDetails('mismatch')).toBe(true)
    expect(showOkSummary('ok')).toBe(true)
    expect(showFailedNote('failed')).toBe(true)
  })
})

describe('formatMissingBuckets', () => {
  it('joins non-empty buckets', () => {
    expect(formatMissingBuckets(['a', 'b'])).toBe('a, b')
  })

  it('returns empty string for empty or nullish', () => {
    expect(formatMissingBuckets([])).toBe('')
    expect(formatMissingBuckets(null)).toBe('')
    expect(formatMissingBuckets(undefined)).toBe('')
  })

  it('hasMissingBuckets is false for empty', () => {
    expect(hasMissingBuckets([])).toBe(false)
    expect(hasMissingBuckets(['x'])).toBe(true)
  })
})

describe('formatLastBackup', () => {
  it('maps null/empty to none sentinel; otherwise returns run_key date', () => {
    expect(formatLastBackup(null)).toBe('none')
    expect(formatLastBackup(undefined)).toBe('none')
    expect(formatLastBackup('')).toBe('none')
    // B12: last_backup is the latest backup ok run_key (Shanghai date)
    expect(formatLastBackup('2026-10-15')).toBe('2026-10-15')
  })
})

describe('formatAccountDriftUsers', () => {
  it('returns null when per-account check is not enabled', () => {
    expect(formatAccountDriftUsers({})).toBeNull()
    expect(
      formatAccountDriftUsers({
        account_check_from: '',
        account_drift_users: 0,
      })
    ).toBeNull()
  })
  it('shows the count when enabled, missing count as 0', () => {
    expect(
      formatAccountDriftUsers({
        account_check_from: '2026-10-16',
        account_drift_users: 3,
      })
    ).toBe('3')
    expect(formatAccountDriftUsers({ account_check_from: '2026-10-16' })).toBe(
      '0'
    )
  })
})
