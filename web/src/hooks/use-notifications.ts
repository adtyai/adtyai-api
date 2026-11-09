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
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { useStatus } from '@/hooks/use-status'
import {
  getNotice,
  getNotificationInbox,
  readNotifications,
  type NotificationInbox,
} from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'
import { useNotificationStore } from '@/stores/notification-store'

const EMPTY_ANNOUNCEMENTS: Record<string, unknown>[] = []
const EMPTY_READ_KEYS: string[] = []

function hashString(input: string): string {
  let hash = 0
  if (!input) return '0'

  for (let i = 0; i < input.length; i += 1) {
    const chr = input.charCodeAt(i)
    hash = (hash << 5) - hash + chr
    hash |= 0
  }

  return hash.toString(36)
}

/**
 * Generate a unique key for an announcement
 * Prefer backend id, fall back to a content hash so edits register
 */
function getAnnouncementKey(item: Record<string, unknown>): string {
  if (!item) return ''

  if (item.id !== undefined && item.id !== null) {
    return `id:${item.id}`
  }

  const fingerprint = JSON.stringify({
    publishDate: (item?.publishDate as string) || '',
    content: ((item?.content as string) || '').trim(),
    extra: ((item?.extra as string) || '').trim(),
    type: (item?.type as string) || '',
    title: ((item?.title as string) || '').trim(),
    link: ((item?.link as string) || '').trim(),
  })
  return `hash:${hashString(fingerprint)}`
}

/**
 * Hook to manage notifications (Notice + Announcements)
 * Provides unread counts and read status management
 */
export function useNotifications() {
  const userID = useAuthStore((state) => state.auth.user?.id ?? 0)
  const queryClient = useQueryClient()
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'notice' | 'announcements'>(
    'notice'
  )

  // Fetch Notice from API
  const {
    data: noticeResponse,
    isLoading: noticeLoading,
    error: noticeError,
    refetch: refetchNotice,
  } = useQuery({
    queryKey: ['notice'],
    queryFn: async () => requireServerSuccess(await getNotice()),
    staleTime: 1000 * 60 * 5, // 5 minutes
    enabled: userID === 0,
  })

  // Fetch Announcements from status
  const { status, loading: statusLoading } = useStatus()
  const announcementsEnabled = status?.announcements_enabled ?? false
  const publicAnnouncements = useMemo<Record<string, unknown>[]>(() => {
    if (!announcementsEnabled) return []
    return ((status?.announcements || []) as Record<string, unknown>[]).slice(
      0,
      20
    )
  }, [announcementsEnabled, status?.announcements])

  const inboxKey = useMemo(
    () => ['adty-notifications', userID, window.location.host] as const,
    [userID]
  )
  const inbox = useInfiniteQuery({
    queryKey: inboxKey,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => getNotificationInbox(pageParam),
    getNextPageParam: (page) => page.next_before_id || undefined,
    enabled: userID > 0,
    staleTime: 30_000,
    refetchInterval: 30_000,
    refetchOnWindowFocus: 'always',
  })
  const read = useMutation({
    mutationFn: (input: { keys: string[]; queryKey: typeof inboxKey }) =>
      readNotifications(input.keys),
    onSuccess: (_data, input) => {
      queryClient.setQueryData<InfiniteData<NotificationInbox>>(
        input.queryKey,
        (previous) => {
          if (!previous) return previous
          const newlyRead = new Set(
            previous.pages.flatMap((page) =>
              page.announcements
                .filter(
                  (item) =>
                    String(item.id).startsWith('publication:') &&
                    input.keys.includes(String(item.read_key)) &&
                    !page.read_keys.includes(String(item.read_key))
                )
                .map((item) => String(item.read_key))
            )
          ).size
          return {
            ...previous,
            pages: previous.pages.map((page) => ({
              ...page,
              unread_publications:
                page.unread_publications === undefined
                  ? undefined
                  : Math.max(0, page.unread_publications - newlyRead),
              read_keys: [...new Set([...page.read_keys, ...input.keys])],
            })),
          }
        }
      )
      void queryClient.invalidateQueries({ queryKey: input.queryKey })
    },
  })
  const markRead = read.mutate
  const accountAnnouncements = useMemo(
    () =>
      inbox.data?.pages.flatMap((page) => page.announcements) ??
      EMPTY_ANNOUNCEMENTS,
    [inbox.data]
  )
  const announcements = userID > 0 ? accountAnnouncements : publicAnnouncements
  const serverReadKeys = useMemo(
    () =>
      inbox.data?.pages.flatMap((page) => page.read_keys) ?? EMPTY_READ_KEYS,
    [inbox.data]
  )
  const firstPage = inbox.data?.pages[0]

  // Notification store
  const {
    lastReadNotice,
    readAnnouncementKeys,
    markNoticeRead,
    markAnnouncementsRead,
  } = useNotificationStore()

  // Extract notice content
  let noticeContent = ''
  if (userID > 0) {
    noticeContent = firstPage?.notice ?? ''
  } else if (noticeResponse?.success) {
    noticeContent = (noticeResponse.data || '').trim()
  }

  // Calculate unread counts
  const unreadCounts = useMemo(() => {
    const noticeRead =
      userID > 0
        ? serverReadKeys.includes(firstPage?.notice_key ?? '')
        : noticeContent === lastReadNotice
    const noticeUnread = noticeContent && !noticeRead ? 1 : 0

    const announcementsUnread =
      announcements.filter((item: Record<string, unknown>) => {
        if (userID > 0) {
          if (
            firstPage?.unread_publications !== undefined &&
            String(item.id).startsWith('publication:')
          ) {
            return false
          }
          return !serverReadKeys.includes(String(item.read_key ?? ''))
        }
        return !readAnnouncementKeys.includes(getAnnouncementKey(item))
      }).length + (userID > 0 ? (firstPage?.unread_publications ?? 0) : 0)

    return {
      notice: noticeUnread,
      announcements: announcementsUnread,
      total: noticeUnread + announcementsUnread,
    }
  }, [
    noticeContent,
    lastReadNotice,
    announcements,
    readAnnouncementKeys,
    userID,
    serverReadKeys,
    firstPage?.notice_key,
    firstPage?.unread_publications,
  ])

  const markAccountTabRead = useCallback(
    (tab: 'notice' | 'announcements') => {
      const keys =
        tab === 'notice'
          ? [firstPage?.notice_key ?? '']
          : announcements.map((item) => String(item.read_key ?? ''))
      const unread = keys.filter((key) => key && !serverReadKeys.includes(key))
      if (unread.length) {
        markRead({ keys: unread.slice(0, 41), queryKey: inboxKey })
      }
    },
    [announcements, firstPage?.notice_key, inboxKey, markRead, serverReadKeys]
  )

  useEffect(() => {
    if (popoverOpen && inbox.isSuccess && !inbox.error && userID > 0) {
      markAccountTabRead(activeTab)
    }
  }, [
    activeTab,
    inbox.error,
    inbox.isSuccess,
    markAccountTabRead,
    popoverOpen,
    userID,
  ])

  const markAnnouncementsAsRead = () => {
    if (announcements.length > 0) {
      const allKeys = announcements.map((item: Record<string, unknown>) =>
        getAnnouncementKey(item)
      )
      markAnnouncementsRead(allKeys)
    }
  }

  // Handle popover open
  const handleOpenPopover = (tab?: 'notice' | 'announcements') => {
    const nextTab = tab || activeTab

    // Mark currently visible content as read when opening the notification center
    if (userID === 0 && noticeContent) {
      markNoticeRead(noticeContent)
    }
    if (userID === 0 && nextTab === 'announcements') {
      markAnnouncementsAsRead()
    }

    setActiveTab(nextTab)
    setPopoverOpen(true)
  }

  const handlePopoverOpenChange = (open: boolean) => {
    if (open) {
      handleOpenPopover(activeTab)
      return
    }

    setPopoverOpen(false)
  }

  // Handle tab change - mark announcements as read when switching to that tab
  const handleTabChange = (tab: 'notice' | 'announcements') => {
    setActiveTab(tab)

    if (userID === 0 && tab === 'announcements') {
      markAnnouncementsAsRead()
    }
  }

  return {
    // Data
    notice: noticeContent,
    announcements,
    hasMore: userID > 0 && inbox.hasNextPage,
    loadingMore: inbox.isFetchingNextPage,
    loadMore: inbox.fetchNextPage,
    loading: userID > 0 ? inbox.isLoading : noticeLoading || statusLoading,
    error: userID > 0 ? inbox.error : noticeError,

    // Unread counts
    unreadCount: unreadCounts.total,
    unreadNoticeCount: unreadCounts.notice,
    unreadAnnouncementsCount: unreadCounts.announcements,

    // Popover state
    popoverOpen,
    setPopoverOpen: handlePopoverOpenChange,
    activeTab,
    setActiveTab: handleTabChange,

    // Actions
    openPopover: handleOpenPopover,
    closePopover: () => setPopoverOpen(false),
    refetchNotice: userID > 0 ? inbox.refetch : refetchNotice,
  }
}
