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
