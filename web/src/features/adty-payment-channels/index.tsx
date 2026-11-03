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
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteEpayChannel,
  epayChannelsQueryKey,
  getEpayChannels,
  setEpayChannelStatus,
  testEpayChannel,
  type EpayChannel,
} from './api'
import { ChannelDrawer } from './components/channel-drawer'

const STATUS_VARIANT = {
  enabled: 'success',
  maintenance: 'warning',
  disabled: 'neutral',
} as const
const METHOD_LABEL: Record<string, string> = {
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
}

/** Alipay / WeChat channels: status, limits, today's amount, tests and switches. */
export function PaymentChannels() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === ROLE.SUPER_ADMIN
  const canOperate = canAdmin(user, ADTY_PERMISSIONS.PAYMENT_OPERATE)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editing, setEditing] = useState<EpayChannel | null>(null)
  const [deleting, setDeleting] = useState<EpayChannel | null>(null)
  const query = useQuery({
    queryKey: epayChannelsQueryKey,
    queryFn: getEpayChannels,
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: epayChannelsQueryKey })

  const status = useMutation({
    mutationFn: (args: { id: number; status: EpayChannel['status'] }) =>
      setEpayChannelStatus(args.id, args.status),
    onSuccess: () => void refresh(),
    onError: (error) => handleServerError(error),
  })
  const test = useMutation({
    mutationFn: (id: number) => testEpayChannel(id),
    onSuccess: (results) => {
      for (const result of results) {
        const label = t(METHOD_LABEL[result.method] ?? result.method)
        if (result.ok) {
          toast.success(
            t('{{method}}: returns {{type}}', {
              method: label,
              type: result.return_type,
            })
          )
          continue
        }
        if (result.disabled) {
          toast.warning(
            t('{{method}} is not open on this channel and was turned off', {
              method: label,
            })
          )
          continue
        }
        toast.error(`${label}: ${result.message}`)
      }
      void refresh()
    },
    onError: (error) => handleServerError(error),
  })
  const remove = useMutation({
    mutationFn: (id: number) => deleteEpayChannel(id),
    onSuccess: () => {
      setDeleting(null)
      void refresh()
    },
    onError: (error) => handleServerError(error),
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Payment channels')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card>
          <CardContent className='flex flex-col gap-3 py-4'>
            {isRoot && (
              <Button
                className='self-start'
                size='sm'
                onClick={() => {
                  setEditing(null)
                  setDrawerOpen(true)
                }}
              >
                <Plus className='mr-2 h-4 w-4' />
                {t('Add payment channel')}
              </Button>
            )}
            <StaticDataTable
              tableClassName='min-w-[1000px]'
              tableProps={{ 'aria-label': t('Payment channels') }}
              data={query.data ?? []}
              getRowKey={(row) => String(row.id)}
              emptyContent={
                <EmptyState
                  className='min-h-32'
                  title={t('No payment channels yet')}
                />
              }
              columns={[
                { id: 'name', header: t('Name'), cell: (row) => row.name },
                {
                  id: 'methods',
                  header: t('Methods'),
                  cell: (row) =>
                    row.methods
                      .filter((m) => m.enabled)
                      .map(
                        (m) =>
                          `${t(METHOD_LABEL[m.method] ?? m.method)}${m.return_type ? ` (${m.return_type})` : ''}`
                      )
                      .join(', ') || '-',
                },
                {
                  id: 'priority',
                  header: t('Priority / weight'),
                  cell: (row) => `${row.priority} / ${row.weight}`,
                },
                {
                  id: 'mode',
                  header: t('Order mode'),
                  cell: (row) => row.order_mode,
                },
                {
                  id: 'status',
                  header: t('Status'),
                  cell: (row) => (
                    <StatusBadge
                      label={`${t(row.status)}${row.status_reason ? ` · ${t(row.status_reason)}` : ''}`}
                      variant={STATUS_VARIANT[row.status]}
                      copyable={false}
                    />
                  ),
                },
                {
                  id: 'unpaid',
                  header: t('Unpaid in a row'),
                  cell: (row) => row.unpaid_streak,
                },
                {
                  id: 'today',
                  header: t('Today'),
                  cell: (row) =>
                    Object.entries(row.today_cny)
                      .map(
                        ([method, sum]) =>
                          `${t(METHOD_LABEL[method] ?? method)} ¥${sum}`
                      )
                      .join(' / ') || '-',
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  className: 'text-right',
                  cellClassName: 'text-right',
                  cell: (row) => (
                    <div className='flex justify-end gap-1'>
                      {canOperate && row.order_mode === 'api' && (
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={test.isPending}
                          onClick={() => test.mutate(row.id)}
                        >
                          {t('Test order')}
                        </Button>
                      )}
                      {canOperate && (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() =>
                            status.mutate({
                              id: row.id,
                              status:
                                row.status === 'enabled'
                                  ? 'disabled'
                                  : 'enabled',
                            })
                          }
                        >
                          {row.status === 'enabled'
                            ? t('Disable')
                            : t('Enable')}
                        </Button>
                      )}
                      {isRoot && (
                        <>
                          <Button
                            size='sm'
                            variant='ghost'
                            onClick={() => {
                              setEditing(row)
                              setDrawerOpen(true)
                            }}
                          >
                            {t('Edit')}
                          </Button>
                          <Button
                            size='sm'
                            variant='ghost'
                            onClick={() => setDeleting(row)}
                          >
                            {t('Delete')}
                          </Button>
                        </>
                      )}
                    </div>
                  ),
                },
              ]}
            />
          </CardContent>
        </Card>
        {isRoot && (
          <ChannelDrawer
            open={drawerOpen}
            channel={editing}
            onOpenChange={setDrawerOpen}
            onSaved={() => void refresh()}
          />
        )}
        <ConfirmDialog
          open={deleting !== null}
          onOpenChange={(open) => !open && setDeleting(null)}
          title={t('Delete payment channel')}
          desc={t(
            'Delete {{name}}? Channels with orders can only be disabled.',
            { name: deleting?.name ?? '' }
          )}
          destructive
          isLoading={remove.isPending}
          handleConfirm={() => deleting && remove.mutate(deleting.id)}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
