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

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { api } from '@/lib/api'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

type LedgerEntry = {
  id: number
  user_id: number
  delta_quota: number
  balance_before: number
  balance_after: number
  type: string
  ref_type: string
  ref_id: string
  reason: string
  operator_id: number
  created_at: number
}

type LedgerPage = { total: number; items: LedgerEntry[] }

const PAGE_SIZE = 20

async function getUserLedger(
  userId: number,
  page: number
): Promise<LedgerPage> {
  const res = await api.get<{ success: boolean; data: LedgerPage }>(
    `/api/adty/admin/users/${userId}/ledger`,
    { params: { p: page, page_size: PAGE_SIZE } }
  )
  return requireServerSuccess(res.data).data
}

/** Balance ledger of one user (adty task 7.2): refunds, manual adjustments, commission transfers. */
export function UserLedgerDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: ['adty-ledger', props.userId, page],
    queryFn: () => getUserLedger(props.userId, page),
    enabled: props.open,
  })
  const typeLabels: Record<string, string> = {
    refund: t('Refund'),
    manual_adjust: t('Manual adjustment'),
    commission_transfer: t('Commission to balance'),
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Balance ledger')}
      contentClassName='sm:max-w-4xl'
    >
      <div className='flex flex-col gap-3'>
        <StaticDataTable
          tableClassName='min-w-[760px]'
          tableProps={{ 'aria-label': t('Balance ledger') }}
          data={query.data?.items ?? []}
          getRowKey={(row) => String(row.id)}
          emptyContent={
            <EmptyState className='min-h-32' title={t('No records')} />
          }
          columns={[
            {
              id: 'time',
              header: t('Time'),
              cell: (row) => formatTimestamp(row.created_at),
            },
            {
              id: 'type',
              header: t('Type'),
              cell: (row) => typeLabels[row.type] ?? row.type,
            },
            {
              id: 'delta',
              header: t('Change'),
              cell: (row) =>
                `${row.delta_quota >= 0 ? '+' : '-'}${formatQuota(Math.abs(row.delta_quota))}`,
            },
            {
              id: 'balance',
              header: t('Balance'),
              cell: (row) =>
                `${formatQuota(row.balance_before)} → ${formatQuota(row.balance_after)}`,
            },
            {
              id: 'reason',
              header: t('Reason'),
              cell: (row) => (
                <span className='text-sm break-words'>
                  {row.reason || row.ref_id || '-'}
                </span>
              ),
            },
            {
              id: 'operator',
              header: t('Operator'),
              cell: (row) =>
                row.operator_id > 0 ? `#${row.operator_id}` : '-',
            },
          ]}
        />
        <PageSwitcher
          page={page}
          total={query.data?.total ?? 0}
          pageSize={PAGE_SIZE}
          onPageChange={setPage}
        />
      </div>
    </Dialog>
  )
}
