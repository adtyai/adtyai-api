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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Skeleton } from '@/components/ui/skeleton'
import type { OwnerCustomer } from '@/features/adty-site-owner/types'
import { ORDER_TRACE_LABELS } from '@/features/recharge-audit/trace-labels'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatQuota, formatTimestamp } from '@/lib/format'

import { getStaffCustomerRecords, STAFF_PAGE_SIZE } from './api'

export function StaffRecordsDialog(props: {
  userId: number
  staffVersion: number
  customer: OwnerCustomer
  kind: 'topups' | 'logs'
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const topups = useQuery({
    queryKey: [
      'adty-staff',
      props.userId,
      props.staffVersion,
      'topups',
      props.customer.id,
      page,
    ],
    queryFn: () => getStaffCustomerRecords('topups', props.customer.id, page),
    enabled: props.kind === 'topups',
    retry: false,
  })
  const usage = useQuery({
    queryKey: [
      'adty-staff',
      props.userId,
      props.staffVersion,
      'logs',
      props.customer.id,
      page,
    ],
    queryFn: () => getStaffCustomerRecords('logs', props.customer.id, page),
    enabled: props.kind === 'logs',
    retry: false,
  })
  const query = props.kind === 'topups' ? topups : usage
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={
        props.kind === 'topups' ? t('Recharge records') : t('Usage records')
      }
      description={props.customer.username}
    >
      {query.isPending && <Skeleton className='h-32' />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {props.kind === 'topups' && topups.data && !topups.isError && (
        <StaticDataTable
          data={topups.data.items ?? []}
          getRowKey={(row) => row.trade_no}
          emptyContent={<EmptyState title={t('No records')} />}
          columns={[
            {
              id: 'number',
              header: t('Order number'),
              cell: (row) => row.trade_no,
            },
            {
              id: 'amount',
              header: t('Credit amount'),
              cell: (row) => formatQuota(row.credit_quota),
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (row) => t(ORDER_TRACE_LABELS[row.status] ?? row.status),
            },
            {
              id: 'time',
              header: t('Time'),
              cell: (row) => formatTimestamp(row.created_at),
            },
          ]}
        />
      )}
      {props.kind === 'logs' && usage.data && !usage.isError && (
        <StaticDataTable
          data={usage.data.items ?? []}
          getRowKey={(row) => row.id}
          emptyContent={<EmptyState title={t('No records')} />}
          columns={[
            { id: 'model', header: t('Model'), cell: (row) => row.model_name },
            {
              id: 'amount',
              header: t('Usage'),
              cell: (row) => formatQuota(row.quota),
            },
            {
              id: 'time',
              header: t('Time'),
              cell: (row) => formatTimestamp(row.created_at),
            },
          ]}
        />
      )}
      {query.data && !query.isError && (
        <PageSwitcher
          page={page}
          total={query.data.total}
          pageSize={STAFF_PAGE_SIZE}
          onPageChange={setPage}
        />
      )}
    </Dialog>
  )
}
