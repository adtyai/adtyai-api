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
import { describe, expect, test } from 'vitest'

import { normalizeTicketStatus, ticketStatusLabel, TICKET_QUEUES } from './ticket-status'

describe('ticket status labels and queues', () => {
  test('keeps 新建, 待分配, and 处理中 as distinct official states', () => {
    expect(normalizeTicketStatus('new')).toBe('new')
    expect(normalizeTicketStatus('pending_assignment')).toBe('pending_assignment')
    expect(normalizeTicketStatus('in_progress')).toBe('in_progress')
    expect(ticketStatusLabel('new')).toBe('Ticket status new')
    expect(ticketStatusLabel('pending_assignment')).toBe('Ticket status pending assignment')
    expect(ticketStatusLabel('in_progress')).toBe('Ticket status in progress')
    expect(new Set(['new', 'pending_assignment', 'in_progress']).size).toBe(3)
  })

  test('maps historical open and assigned without collapsing 待分配 into 处理中', () => {
    expect(normalizeTicketStatus('open')).toBe('new')
    expect(normalizeTicketStatus('open', 0)).toBe('new')
    expect(normalizeTicketStatus('open', 9)).toBe('in_progress')
    expect(normalizeTicketStatus('assigned')).toBe('in_progress')
    expect(ticketStatusLabel('open')).toBe('Ticket status new')
    expect(ticketStatusLabel('assigned', 4)).toBe('Ticket status in progress')
    expect(ticketStatusLabel('open', 4)).toBe('Ticket status in progress')
  })

  test('exposes official workbench queues', () => {
    expect(TICKET_QUEUES.map((queue) => queue.id)).toEqual([
      'pending',
      'waiting_user',
      'in_progress',
      'urgent',
    ])
  })
})
