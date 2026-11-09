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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getPublications,
  notificationManagerKey,
  withdrawPublication,
  type NotificationMode,
  type Publication,
} from '../api'
import {
  notificationAudienceLabels,
  notificationKindLabels,
} from '../lib/schema'
import { NotificationEditor } from './notification-editor'

function NotificationManagement(props: {
  mode: NotificationMode
  siteId: number
  userId: number
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<{ row: Publication | null } | null>(
    null
  )
  const [withdrawing, setWithdrawing] = useState<Publication | null>(null)
  const query = useQuery({
    queryKey: [
      ...notificationManagerKey,
      props.userId,
      props.mode,
      props.siteId,
      window.location.host,
      page,
    ],
    queryFn: () => getPublications(props.mode, page),
  })
  const withdraw = useMutation({
    mutationFn: (row: Publication) => {
      const user = useAuthStore.getState().auth.user
      if (
        user?.id !== props.userId ||
        (props.mode === 'root' && user.role < ROLE.SUPER_ADMIN)
      ) {
        throw new Error(t('Unauthorized, insufficient privileges'))
      }
      return withdrawPublication(props.mode, row)
    },
    onSuccess: () => {
      setWithdrawing(null)
      void client.invalidateQueries({ queryKey: notificationManagerKey })
      void client.invalidateQueries({ queryKey: ['adty-notifications'] })
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t(
            props.mode === 'root'
              ? 'Targeted notifications'
              : 'Site announcements'
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        <div>
          <Button size='sm' onClick={() => setEditing({ row: null })}>
            {t('New notification')}
          </Button>
        </div>
        {query.isLoading && <LoadingState />}
        {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
        {query.isSuccess && (
          <>
            <StaticDataTable
              tableClassName='min-w-[640px]'
              tableProps={{ 'aria-label': t('Published notifications') }}
              data={query.data.items ?? []}
              getRowKey={(row) => String(row.id)}
              emptyContent={
                <EmptyState
                  className='min-h-24'
                  title={t('No notifications yet')}
                />
              }
              columns={[
                {
                  id: 'title',
                  header: t('Title'),
                  cell: (row) => (
                    <span className='block max-w-64 truncate' title={row.title}>
                      {row.title}
                    </span>
                  ),
                },
                {
                  id: 'kind',
                  header: t('Notification type'),
                  cell: (row) => t(notificationKindLabels[row.kind]),
                },
                {
                  id: 'recipients',
                  header: t('Recipients'),
                  cell: (row) =>
                    row.audience === 'site'
                      ? row.target_site_name
                      : t(notificationAudienceLabels[row.audience]),
                },
                {
                  id: 'publisher',
                  header: t('Publisher'),
                  cell: (row) => row.publisher_name,
                },
                {
                  id: 'published',
                  header: t('Published at'),
                  cell: (row) => formatTimestamp(row.published_at),
                },
                {
                  id: 'status',
                  header: t('Status'),
                  cell: (row) => (
                    <StatusBadge
                      label={t(
                        row.status === 'withdrawn' ? 'Withdrawn' : 'Published'
                      )}
                      variant={
                        row.status === 'withdrawn' ? 'neutral' : 'success'
                      }
                    />
                  ),
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  cell: (row) =>
                    row.status === 'published' && (
                      <div className='flex gap-2'>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setEditing({ row })}
                        >
                          {t('Edit')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setWithdrawing(row)}
                        >
                          {t('Withdraw notification')}
                        </Button>
                      </div>
                    ),
                },
              ]}
            />
            <PageSwitcher
              page={page}
              pageSize={20}
              total={query.data.total}
              onPageChange={setPage}
            />
          </>
        )}
        {editing && (
          <NotificationEditor
            {...props}
            row={editing.row}
            onClose={() => setEditing(null)}
          />
        )}
        <ConfirmDialog
          open={!!withdrawing}
          onOpenChange={(open) => {
            if (!open && !withdraw.isPending) setWithdrawing(null)
          }}
          title={t('Withdraw notification')}
          desc={t(
            'Recipients will no longer see this notification. Read records are retained.'
          )}
          destructive
          isLoading={withdraw.isPending}
          handleConfirm={() => {
            if (withdrawing) withdraw.mutate(withdrawing)
          }}
        />
      </CardContent>
    </Card>
  )
}

export function NotificationManager(props: {
  mode: NotificationMode
  siteId?: number
}) {
  const user = useAuthStore((state) => state.auth.user)
  if (!user || (props.mode === 'root' && user.role < ROLE.SUPER_ADMIN)) {
    return null
  }
  return (
    <NotificationManagement
      key={`${user.id}:${props.siteId ?? 0}`}
      mode={props.mode}
      siteId={props.siteId ?? 0}
      userId={user.id}
    />
  )
}
