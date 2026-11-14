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
*/
import { describe, expect, it } from 'vitest'

import {
  ORDER_MANAGEMENT_BADGE_KINDS,
  REFERRAL_ADMIN_BADGE_KINDS,
  TICKET_BADGE_KINDS,
  UNCONFIRMED_SIDEBAR_BADGE_KINDS,
  sidebarBadgeText,
} from '../use-sidebar-badges'

describe('sidebarBadgeText', () => {
  const labels = { mixedLabel: 'To-do', failedLabel: 'Unavailable' }

  it('uses a real backend count and does not invent unconfirmed kinds', () => {
    const counts = {
      payment_orphans_pending_review: 2,
      tickets_pending: 1,
      topup_orders_pending: 9,
    }
    expect(
      sidebarBadgeText(counts, ORDER_MANAGEMENT_BADGE_KINDS, labels)
    ).toBe('2')
    expect(ORDER_MANAGEMENT_BADGE_KINDS).not.toContain('topup_orders_pending')
    for (const kind of UNCONFIRMED_SIDEBAR_BADGE_KINDS) {
      expect(TICKET_BADGE_KINDS).not.toContain(kind)
      expect(ORDER_MANAGEMENT_BADGE_KINDS).not.toContain(kind)
      expect(REFERRAL_ADMIN_BADGE_KINDS).not.toContain(kind)
    }
  })

  it('keeps notification unread distinct from finance and ticket badges', () => {
    const counts = {
      notification_unread: 4,
      payment_orphans_pending_review: 1,
      tickets_waiting_user: 2,
    }
    expect(
      sidebarBadgeText(counts, ORDER_MANAGEMENT_BADGE_KINDS, labels)
    ).toBe('1')
    expect(sidebarBadgeText(counts, TICKET_BADGE_KINDS, labels)).toBe('2')
    expect(
      sidebarBadgeText(counts, REFERRAL_ADMIN_BADGE_KINDS, labels)
    ).toBeUndefined()
  })

  it('does not sum different duties and does not treat a failed query as zero', () => {
    const counts = { tickets_pending: 3, tickets_waiting_user: 1 }
    expect(sidebarBadgeText(counts, TICKET_BADGE_KINDS, labels)).toBe('To-do')
    expect(
      sidebarBadgeText(counts, TICKET_BADGE_KINDS, { ...labels, failed: true })
    ).toBe('Unavailable')
    expect(
      sidebarBadgeText({ tickets_pending: 3 }, TICKET_BADGE_KINDS, {
        ...labels,
        failed: true,
      })
    ).toBe('Unavailable')
    expect(sidebarBadgeText(undefined, TICKET_BADGE_KINDS, labels)).toBeUndefined()
    expect(
      sidebarBadgeText({ tickets_pending: 0 }, TICKET_BADGE_KINDS, labels)
    ).toBeUndefined()
  })
})
