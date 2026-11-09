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
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestamp } from '@/lib/format'

import { adtyTopupQueryKeys, getAdtyTopupHistory } from '../api'
import {
  methodLabel,
  ORDER_STATUS_LABELS,
  ORDER_STATUS_VARIANTS,
} from '../lib/methods'
import { formatUsd } from '../lib/money'

const PAGE_SIZE = 10

/** The user's own top-up orders with face value, bonus, payment and status. */
export function AdtyTopupHistory() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: adtyTopupQueryKeys.history(page),
    queryFn: () => getAdtyTopupHistory(page, PAGE_SIZE),
  })
  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }

  return (
    <Card id='wallet-history' className='scroll-mt-4'>
      <CardHeader>
        <CardTitle>{t('Top-up history')}</CardTitle>
        <CardDescription>
          {t('Orders paid after they expire are still credited.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
        <StaticDataTable
          tableClassName='min-w-[720px]'
          tableProps={{ 'aria-label': t('Top-up history') }}
          data={items}
          getRowKey={(row) => row.trade_no}
          emptyContent={
            <EmptyState
              className='min-h-32'
              title={query.isLoading ? t('Loading...') : t('No top-ups yet')}
            />
          }
          columns={[
            {
              id: 'time',
              header: t('Time'),
              cell: (row) => formatTimestamp(row.create_time),
            },
            {
              id: 'method',
              header: t('Payment method'),
              cell: (row) => t(methodLabel(row.payment_method)),
            },
            {
              id: 'scene',
              header: t('Payment scene'),
              cell: (row) =>
                row.ext?.payment_scene
                  ? `${row.ext.payment_scene_name || row.ext.payment_scene} (${row.ext.payment_scene}) · v${row.ext.payment_scene_version}`
                  : '-',
            },
            {
              id: 'face',
              header: t('Amount'),
              cell: (row) =>
                row.ext ? formatUsd(Number(row.ext.face_usd), locale) : '-',
            },
            {
              id: 'bonus',
              header: t('Bonus'),
              cell: (row) => {
                if (!row.ext) return '-'
                const bonus =
                  Number(row.ext.bonus_usd) +
                  Number(row.ext.promo_bonus_usd || 0)
                return formatUsd(bonus, locale)
              },
            },
            {
              id: 'paid',
              header: t('Paid'),
              cell: (row) => {
                if (!row.ext) return '-'
                const paid = Number(row.ext.paid_amount)
                const value =
                  paid > 0 ? row.ext.paid_amount : row.ext.pay_amount
                return `${value} ${row.ext.paid_currency || row.ext.pay_currency}`
              },
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (row) => (
                <StatusBadge
                  label={t(ORDER_STATUS_LABELS[row.status] ?? row.status)}
                  variant={ORDER_STATUS_VARIANTS[row.status] ?? 'neutral'}
                  copyable={false}
                />
              ),
            },
            {
              id: 'trade_no',
              header: t('Order number'),
              cell: (row) => (
                <code className='font-mono text-xs'>{row.trade_no}</code>
              ),
            },
          ]}
        />
        {pages > 1 && (
          <div className='flex items-center justify-end gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-sm'>
              {page} / {pages}
            </span>
            <Button
              variant='outline'
              size='sm'
              disabled={page >= pages}
              onClick={() => setPage((p) => p + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
