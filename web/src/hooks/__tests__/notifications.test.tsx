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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'
import { useNotificationStore } from '@/stores/notification-store'

import { useNotifications } from '../use-notifications'

const clients: QueryClient[] = []

beforeEach(() => {
  localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  useAuthStore.getState().auth.reset()
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

describe('account notification receipts', () => {
  const noticeKey = '1'.repeat(64)
  const announcementKey = '2'.repeat(64)
  function mountAccountNotifications() {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    clients.push(client)
    client.setQueryData(STATUS_QUERY_KEY, {})
    client.setQueryData(['notice'], { success: true, data: '' })
    const wrapper = (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
    return { client, wrapper }
  }

  function account(id: number) {
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id, username: `user${id}`, role: 1 })
    )
  }

  it('loads the next page without losing the first page or its receipts', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockImplementation(async (_url, config) => ({
        data: {
          success: true,
          data: {
            notice: '',
            notice_key: '',
            read_keys: [],
            unread_publications: 22,
            announcements: [
              {
                id: (config?.params as { before_id?: number })?.before_id
                  ? 'publication:11'
                  : 'publication:12',
                title: (config?.params as { before_id?: number })?.before_id
                  ? 'Older notification'
                  : 'Newer notification',
                read_key: (config?.params as { before_id?: number })?.before_id
                  ? announcementKey
                  : noticeKey,
              },
            ],
            next_before_id: (config?.params as { before_id?: number })
              ?.before_id
              ? 0
              : 12,
          },
        },
      }))
    account(101)
    const mounted = mountAccountNotifications()
    const panel = renderHook(() => useNotifications(), {
      wrapper: mounted.wrapper,
    })
    await waitFor(() =>
      expect(panel.result.current.announcements).toHaveLength(1)
    )
    expect(panel.result.current.hasMore).toBe(true)
    expect(panel.result.current.unreadCount).toBe(22)
    await act(async () => {
      await panel.result.current.loadMore()
    })
    await waitFor(() =>
      expect(panel.result.current.announcements).toHaveLength(2)
    )
    expect(panel.result.current.hasMore).toBe(false)
    expect(panel.result.current.unreadCount).toBe(22)
    expect(get).toHaveBeenLastCalledWith('/api/adty/notifications', {
      params: { before_id: 12 },
    })
  })

  it('isolates A and B, ignores anonymous receipts, and reads the same account on a fresh device', async () => {
    const readByUser = new Map<number, string[]>()
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      expect(url).toBe('/api/adty/notifications')
      return {
        data: {
          success: true,
          data: {
            notice: 'Account notice',
            notice_key: noticeKey,
            announcements: [
              {
                id: 11,
                content: 'Account announcement',
                read_key: announcementKey,
              },
            ],
            read_keys:
              readByUser.get(useAuthStore.getState().auth.user?.id ?? 0) ?? [],
          },
        },
      }
    })
    const post = vi.spyOn(api, 'post').mockImplementation(async (url, data) => {
      expect(url).toBe('/api/adty/notifications/read')
      const keys = (data as { keys: string[] }).keys
      readByUser.set(useAuthStore.getState().auth.user?.id ?? 0, keys)
      return { data: { success: true, data: null } }
    })
    useNotificationStore.getState().markNoticeRead('Account notice')
    useNotificationStore.getState().markAnnouncementsRead(['id:11'])
    account(101)
    const first = mountAccountNotifications()
    const panel = renderHook(() => useNotifications(), {
      wrapper: first.wrapper,
    })
    const badge = renderHook(() => useNotifications(), {
      wrapper: first.wrapper,
    })
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(2))
    act(() => panel.result.current.openPopover('notice'))
    await waitFor(() => expect(badge.result.current.unreadCount).toBe(1))
    expect(post).toHaveBeenCalledWith('/api/adty/notifications/read', {
      keys: [noticeKey],
    })
    expect(panel.result.current.unreadAnnouncementsCount).toBe(1)
    act(() => panel.result.current.closePopover())
    account(202)
    await waitFor(() => expect(badge.result.current.unreadCount).toBe(2))
    account(101)
    await waitFor(() => expect(badge.result.current.unreadCount).toBe(1))
    const fresh = mountAccountNotifications()
    const device = renderHook(() => useNotifications(), {
      wrapper: fresh.wrapper,
    })
    await waitFor(() => expect(device.result.current.unreadCount).toBe(1))
    expect(device.result.current.unreadNoticeCount).toBe(0)
    expect(device.result.current.unreadAnnouncementsCount).toBe(1)
  })

  it('keeps failed reads unread and detects an edited announcement with the same ID', async () => {
    let revision = announcementKey
    const readKeys: string[] = []
    vi.spyOn(api, 'get').mockImplementation(async () => ({
      data: {
        success: true,
        data: {
          notice: '',
          notice_key: '',
          announcements: [
            { id: 11, content: `Revision ${revision}`, read_key: revision },
          ],
          read_keys: [...readKeys],
        },
      },
    }))
    const post = vi
      .spyOn(api, 'post')
      .mockRejectedValueOnce(new Error('Temporary receipt failure'))
      .mockImplementation(async (_url, data) => {
        readKeys.push(...(data as { keys: string[] }).keys)
        return { data: { success: true, data: null } }
      })
    account(101)
    const mounted = mountAccountNotifications()
    const panel = renderHook(() => useNotifications(), {
      wrapper: mounted.wrapper,
    })
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(1))
    act(() => panel.result.current.openPopover('announcements'))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(panel.result.current.unreadCount).toBe(1)
    act(() => panel.result.current.closePopover())
    act(() => panel.result.current.openPopover('announcements'))
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(0))
    act(() => panel.result.current.closePopover())
    revision = '3'.repeat(64)
    await act(async () => {
      await mounted.client.invalidateQueries({
        queryKey: ['adty-notifications'],
      })
    })
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(1))
  })

  it('marks the selected tab after an inbox resolves while the center is open', async () => {
    let finish: (() => void) | undefined
    vi.spyOn(api, 'get').mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = () =>
            resolve({
              data: {
                success: true,
                data: {
                  notice: 'Account notice',
                  notice_key: noticeKey,
                  announcements: [],
                  read_keys: [],
                },
              },
            })
        })
    )
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: null },
    })
    account(101)
    const mounted = mountAccountNotifications()
    const panel = renderHook(() => useNotifications(), {
      wrapper: mounted.wrapper,
    })
    act(() => panel.result.current.openPopover('notice'))
    await act(async () => {
      finish?.()
    })
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/adty/notifications/read', {
        keys: [noticeKey],
      })
    )
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(0))
  })

  it('keeps a late acknowledgment bound to the account that submitted it', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          notice: 'Shared notice',
          notice_key: noticeKey,
          announcements: [],
          read_keys: [],
        },
      },
    })
    let finish: (() => void) | undefined
    const post = vi.spyOn(api, 'post').mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = () => resolve({ data: { success: true, data: null } })
        })
    )
    account(101)
    const mounted = mountAccountNotifications()
    const panel = renderHook(() => useNotifications(), {
      wrapper: mounted.wrapper,
    })
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(1))
    act(() => panel.result.current.openPopover('notice'))
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    account(202)
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(1))
    await act(async () => {
      finish?.()
    })
    expect(panel.result.current.unreadCount).toBe(1)
  })

  it('exposes an inbox fetch failure and retries without inheriting anonymous receipts', async () => {
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('Inbox unavailable'))
      .mockResolvedValue({
        data: {
          success: true,
          data: {
            notice: 'Shared notice',
            notice_key: noticeKey,
            announcements: [],
            read_keys: [],
          },
        },
      })
    useNotificationStore.getState().markNoticeRead('Shared notice')
    account(101)
    const mounted = mountAccountNotifications()
    const panel = renderHook(() => useNotifications(), {
      wrapper: mounted.wrapper,
    })
    await waitFor(() =>
      expect(panel.result.current.error).toBeInstanceOf(Error)
    )
    await act(async () => {
      await panel.result.current.refetchNotice()
    })
    await waitFor(() => expect(panel.result.current.unreadCount).toBe(1))
    expect(panel.result.current.error).toBeNull()
  })
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
