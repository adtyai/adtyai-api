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
export const TICKET_STATUS_LABELS: Record<string, string> = {
  new: 'Ticket status new',
  pending_assignment: 'Ticket status pending assignment',
  in_progress: 'Ticket status in progress',
  waiting_user: 'Ticket status waiting user',
  waiting_internal: 'Ticket status waiting internal',
  resolved: 'Ticket status resolved',
  closed: 'Ticket status closed',
}

export type TicketQueueId = 'pending' | 'waiting_user' | 'in_progress' | 'urgent'

export const TICKET_QUEUES: { id: TicketQueueId; label: string; badge: keyof TicketBadges }[] = [
  { id: 'pending', label: 'Ticket queue pending', badge: 'pending' },
  { id: 'waiting_user', label: 'Ticket queue waiting user', badge: 'waiting_user' },
  { id: 'in_progress', label: 'Ticket queue in progress', badge: 'in_progress' },
  { id: 'urgent', label: 'Ticket queue urgent', badge: 'urgent' },
]

export type TicketBadges = {
  new: number
  pending_assignment: number
  in_progress: number
  waiting_user: number
  waiting_internal: number
  urgent: number
  pending: number
}

export function normalizeTicketStatus(status: string, assigneeId = 0): string {
  if (status === 'assigned') return 'in_progress'
  if (status === 'open') return assigneeId > 0 ? 'in_progress' : 'new'
  return status
}

export function ticketStatusLabel(status: string, assigneeId = 0): string {
  const official = normalizeTicketStatus(status, assigneeId)
  return TICKET_STATUS_LABELS[official] ?? official
}
