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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useNotificationStore } from '@/stores/notification-store'

import { useNotifications } from '../use-notifications'

const clients: QueryClient[] = []

beforeEach(() => {
  localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  localStorage.clear()
})

describe('announcement badges', () => {
  it.each([
    { name: 'server IDs', first: { id: 11, content: 'First announcement' } },
    { name: 'legacy content keys', first: { content: 'First announcement' } },
  ])(
    'updates every mounted badge after reading $name and counts later announcements',
    async ({ first }) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      clients.push(client)
      client.setQueryData(STATUS_QUERY_KEY, {
        announcements_enabled: true,
        announcements: [first],
      })
      client.setQueryData(['notice'], { success: true, data: '' })
      const wrapper = (props: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          {props.children}
        </QueryClientProvider>
      )
      const panel = renderHook(() => useNotifications(), { wrapper })
      const header = renderHook(() => useNotifications(), { wrapper })
      expect(panel.result.current.unreadAnnouncementsCount).toBe(1)
      expect(header.result.current.unreadCount).toBe(1)

      act(() => panel.result.current.setActiveTab('announcements'))

      expect(panel.result.current.unreadAnnouncementsCount).toBe(0)
      expect(header.result.current.unreadCount).toBe(0)

      act(() => {
        client.setQueryData(STATUS_QUERY_KEY, {
          announcements_enabled: true,
          announcements: [first, { id: 12, content: 'Later announcement' }],
        })
      })
      await waitFor(() => expect(header.result.current.unreadCount).toBe(1))
      expect(panel.result.current.unreadAnnouncementsCount).toBe(1)

      act(() => panel.result.current.openPopover('announcements'))

      expect(header.result.current.unreadCount).toBe(0)
      expect(panel.result.current.unreadAnnouncementsCount).toBe(0)
    }
  )
})
