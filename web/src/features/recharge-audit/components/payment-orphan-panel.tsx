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
import { toast } from 'sonner'
import { invalidateSidebarBadges } from '@/hooks/use-sidebar-badges'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  creditPaymentOrphan,
  getPaymentOrphans,
  type PaymentOrphanEvent,
} from '../api'
import { PaymentOrphanReviewDialog } from './payment-orphan-review-dialog'

type PaymentOrphanPanelProps = {
  canCredit: boolean
}

/** Payment callbacks that did not credit an order, waiting for an operator. */
export function PaymentOrphanPanel(props: PaymentOrphanPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [status, setStatus] = useState('pending_review')
  const [reviewEvent, setReviewEvent] = useState<PaymentOrphanEvent | null>(
    null
  )
  const query = useQuery({
    queryKey: ['recharge-audit', 'orphans', status],
    queryFn: () => getPaymentOrphans(status, 1),
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['recharge-audit'] })
    void invalidateSidebarBadges(queryClient)
  const credit = useMutation({
    mutationFn: (id: number) => creditPaymentOrphan(id, ''),
    onSuccess: () => {
      toast.success(t('Order credited'))
      void refresh()
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <>
      <Card>
        <CardHeader className='flex flex-row flex-wrap items-start justify-between gap-3'>
          <div>
            <CardTitle>{t('Payments needing review')}</CardTitle>
            <CardDescription>
              {t(
                'Callbacks that could not credit an order. Amount mismatches must be completed manually.'
              )}
            </CardDescription>
          </div>
          <div className='flex gap-1'>
            {['pending_review', 'all'].map((value) => (
              <Button
                key={value}
                size='sm'
                variant={status === value ? 'default' : 'outline'}
                onClick={() => setStatus(value)}
              >
                {value === 'all' ? t('All') : t('Pending review')}
              </Button>
            ))}
          </div>
        </CardHeader>
        <CardContent>
          {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
          {query.isPending ? (
            <LoadingState />
          ) : (
            query.data && (
              <StaticDataTable
                tableClassName='min-w-[760px]'
                tableProps={{ 'aria-label': t('Payments needing review') }}
                data={query.data?.items ?? []}
                getRowKey={(row) => String(row.id)}
                emptyContent={
                  <EmptyState
                    className='min-h-24'
                    title={t('Nothing to review')}
                  />
                }
                columns={[
                  {
                    id: 'time',
                    header: t('Time'),
                    cell: (row) => formatTimestamp(row.create_time),
                  },
                  {
                    id: 'provider',
                    header: t('Provider'),
                    cell: (row) => row.provider,
                  },
                  {
                    id: 'order',
                    header: t('Order number'),
                    cell: (row) => (
                      <code className='font-mono text-xs'>
                        {row.reference_id}
                      </code>
                    ),
                  },
                  {
                    id: 'reason',
                    header: t('Reason'),
                    cell: (row) => (
                      <span className='text-muted-foreground text-xs'>
                        {row.error || row.reason}
                      </span>
                    ),
                  },
                  {
                    id: 'status',
                    header: t('Status'),
                    cell: (row) => (
                      <StatusBadge
                        label={row.status}
                        variant='neutral'
                        copyable={false}
                      />
                    ),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    className: 'text-right',
                    cellClassName: 'text-right',
                    cell: (row) =>
                      (props.canCredit && row.status === 'pending_review') ||
                      row.review ? (
                        <div className='flex justify-end gap-1'>
                          {props.canCredit &&
                            row.status === 'pending_review' && (
                              <Button
                                size='sm'
                                disabled={!row.can_credit || credit.isPending}
                                onClick={() => credit.mutate(row.id)}
                              >
                                {t('Credit')}
                              </Button>
                            )}
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => setReviewEvent(row)}
                          >
                            {t(
                              props.canCredit && row.status === 'pending_review'
                                ? 'Review'
                                : 'View review'
                            )}
                          </Button>
                        </div>
                      ) : null,
                  },
                ]}
              />
            )
          )}
        </CardContent>
      </Card>
      {reviewEvent && (
        <PaymentOrphanReviewDialog
          key={reviewEvent.id}
          event={reviewEvent}
          canReview={props.canCredit}
          onClose={() => setReviewEvent(null)}
          onDone={() => void refresh()}
        />
      )}
    </>
  )
}
