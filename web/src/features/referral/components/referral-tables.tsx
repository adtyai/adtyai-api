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

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  cancelReferralWithdrawal,
  getReferralCommissions,
  getReferralWithdrawals,
  REFERRAL_PAGE_SIZE,
  referralQueryKeys,
} from '../api'
import { usdtNetworkLabel } from '../lib/address'
import {
  COMMISSION_STATUS_LABELS,
  COMMISSION_STATUS_VARIANTS,
  WITHDRAWAL_STATUS_LABELS,
  WITHDRAWAL_STATUS_VARIANTS,
  WITHDRAWAL_TARGET_LABELS,
} from '../lib/labels'
import type { ReferralWithdrawal } from '../types'
import { PageSwitcher } from './page-switcher'

export function CommissionTable() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: referralQueryKeys.commissions(page),
    queryFn: () => getReferralCommissions(page),
  })
  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }

  return (
    <div className='flex flex-col gap-3'>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <StaticDataTable
        tableProps={{ 'aria-label': t('Commission details') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState className='min-h-32' title={t('No commission yet')} />
        }
        columns={[
          {
            id: 'time',
            header: t('Time'),
            cell: (row) => formatTimestamp(row.created_at),
          },
          {
            id: 'order',
            header: t('Top-up'),
            cell: (row) => formatUsd(row.base_amount, locale),
          },
          { id: 'rate', header: t('Rate'), cell: (row) => `${row.rate}%` },
          {
            id: 'amount',
            header: t('Commission'),
            cell: (row) => formatUsd(row.commission_amount, locale),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <StatusBadge
                label={t(COMMISSION_STATUS_LABELS[row.status])}
                variant={COMMISSION_STATUS_VARIANTS[row.status]}
              />
            ),
          },
          {
            id: 'settle',
            header: t('Settles at'),
            cell: (row) =>
              row.status === 'pending' ? formatTimestamp(row.settle_at) : '-',
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={REFERRAL_PAGE_SIZE}
        onPageChange={setPage}
      />
    </div>
  )
}

/** Where the money goes: chain + address, payee, or balance. */
function withdrawalTarget(row: ReferralWithdrawal): string {
  if (row.account_type === 'usdt') {
    return `${usdtNetworkLabel(row.account_network)} ${row.account_no}`
  }
  if (row.account_type === 'balance') return ''
  return row.account_name
}

export function WithdrawalTable(props: { cancelWindowMinutes: number }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: referralQueryKeys.withdrawals(page),
    queryFn: () => getReferralWithdrawals(page),
  })
  const cancel = useMutation({
    mutationFn: cancelReferralWithdrawal,
    onSuccess: () => {
      toast.success(t('Withdrawal canceled'))
      void queryClient.invalidateQueries({ queryKey: referralQueryKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }
  const now = Date.now() / 1000
  return (
    <div className='flex flex-col gap-3'>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <StaticDataTable
        tableClassName='min-w-[860px]'
        tableProps={{ 'aria-label': t('Withdrawal records') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState className='min-h-32' title={t('No withdrawals yet')} />
        }
        columns={[
          {
            id: 'time',
            header: t('Time'),
            cell: (row) => formatTimestamp(row.submitted_at),
          },
          {
            id: 'target',
            header: t('Withdraw to'),
            cell: (row) => (
              <div className='flex flex-col'>
                <span>{t(WITHDRAWAL_TARGET_LABELS[row.account_type])}</span>
                <span className='text-muted-foreground font-mono text-xs break-all'>
                  {withdrawalTarget(row)}
                </span>
              </div>
            ),
          },
          {
            id: 'amount',
            header: t('Amount'),
            cell: (row) => {
              if (row.account_type === 'usdt') {
                return t('{{amount}} → {{net}} USDT', {
                  amount: formatUsd(row.amount, locale),
                  net: row.net_amount.toFixed(2),
                })
              }
              if (row.cny_amount) {
                return `${formatUsd(row.amount, locale)} → ¥${row.cny_amount}`
              }
              return formatUsd(row.amount, locale)
            },
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <div className='flex flex-col gap-1'>
                <StatusBadge
                  label={t(WITHDRAWAL_STATUS_LABELS[row.status])}
                  variant={WITHDRAWAL_STATUS_VARIANTS[row.status]}
                />
                {row.reject_reason && (
                  <span className='text-muted-foreground text-xs'>
                    {row.reject_reason}
                  </span>
                )}
              </div>
            ),
          },
          {
            id: 'txn',
            header: t('Transaction hash'),
            cell: (row) => (
              <span className='font-mono text-xs break-all'>
                {row.payment_txn_no || '-'}
              </span>
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (row) =>
              row.status === 'pending' &&
              now - row.submitted_at < props.cancelWindowMinutes * 60 ? (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={cancel.isPending}
                  onClick={() => cancel.mutate(row.id)}
                >
                  {t('Cancel')}
                </Button>
              ) : null,
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={REFERRAL_PAGE_SIZE}
        onPageChange={setPage}
      />
    </div>
  )
}
