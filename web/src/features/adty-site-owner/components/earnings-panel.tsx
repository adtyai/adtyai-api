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
import { useMutation, useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

import {
  downloadExport,
  getOwnerEarningLines,
  getOwnerEarnings,
  OWNER_PAGE_SIZE,
  siteOwnerKeys,
  type ExportKind,
} from '../api'
import { isNegativeAmount } from '../lib/site-owner'
import type { EarningKind, EarningRow } from '../types'
import { OwnerStatGrid } from './stat-grid'

const EARNING_KINDS: EarningKind[] = [
  'usage',
  'refund',
  'bonus',
  'bonus_reversal',
  'trial',
  'skipped',
]

/** Literal t() calls so the key scanner sees every label. */
function kindLabel(t: TFunction, kind: EarningKind): string {
  switch (kind) {
    case 'usage':
      return t('Usage')
    case 'refund':
      return t('Refund')
    case 'bonus':
      return t('Bonus you paid')
    case 'bonus_reversal':
      return t('Bonus returned')
    case 'trial':
      return t('Trial credit')
    case 'skipped':
      return t('Not counted')
  }
}

function reasonLabel(t: TFunction, reason: string): string {
  if (reason === 'self_use') return t('Suspected self-use')
  if (reason === 'migration') return t('Suspected move from the main site')
  return reason
}

function statusLabel(t: TFunction, status: EarningRow['status']): string {
  if (status === 'available') return t('Settled')
  if (status === 'void') return t('Void')
  return t('Pending')
}

/** Earnings ledger with balances; negative amounts are shown in red (task 11.6). */
export function EarningsPanel() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [page, setPage] = useState(1)
  const [kind, setKind] = useState('')
  const query = useQuery({
    queryKey: siteOwnerKeys.list('earnings', { p: page, kind }),
    queryFn: () => getOwnerEarnings(page, kind),
  })
  const lines = useQuery({
    queryKey: siteOwnerKeys.list('earning-lines', {}),
    queryFn: getOwnerEarningLines,
  })
  const exporter = useMutation({
    mutationFn: (exportKind: ExportKind) => downloadExport(exportKind),
    onError: (err) => handleServerError(err),
  })
  const balances = query.data?.balances
  const money = (value: string | undefined) =>
    formatUsd(Number(value ?? 0), locale)
  const amountCell = (value: string) => (
    <span
      data-negative={isNegativeAmount(value) ? 'true' : undefined}
      className={cn(isNegativeAmount(value) && 'text-destructive')}
    >
      {money(value)}
    </span>
  )

  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }

  return (
    <div className='flex flex-col gap-4'>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <OwnerStatGrid
        label={t('Balances')}
        items={[
          {
            label: t('Pending settlement'),
            value: money(balances?.pending_usd),
            negative: isNegativeAmount(balances?.pending_usd),
          },
          {
            label: t('Withdrawable'),
            value: money(balances?.available_usd),
            negative: isNegativeAmount(balances?.available_usd),
          },
          { label: t('Withdrawn'), value: money(balances?.withdrawn_usd) },
          { label: t('Not counted'), value: money(balances?.skipped_usd) },
        ]}
      />
      <p className='text-muted-foreground text-sm'>
        {t(
          'Withdraw site earnings using Earnings wallet withdrawal on this tab. A negative balance cannot be withdrawn.'
        )}
      </p>
      <div className='flex flex-wrap items-center gap-2'>
        <NativeSelect
          aria-label={t('Type')}
          value={kind}
          onChange={(event) => {
            setKind(event.target.value)
            setPage(1)
          }}
        >
          <NativeSelectOption value=''>{t('All types')}</NativeSelectOption>
          {EARNING_KINDS.map((key) => (
            <NativeSelectOption key={key} value={key}>
              {kindLabel(t, key)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        {(
          [
            ['earnings', t('Export earnings')],
            ['stats', t('Export report')],
            ['customers', t('Export customers')],
          ] as const
        ).map(([exportKind, label]) => (
          <Button
            key={exportKind}
            size='sm'
            variant='outline'
            disabled={exporter.isPending}
            onClick={() => exporter.mutate(exportKind)}
          >
            {label}
          </Button>
        ))}
      </div>
      <StaticDataTable
        tableClassName='min-w-[720px]'
        tableProps={{ 'aria-label': t('Earnings') }}
        data={query.data?.page.items ?? []}
        getRowKey={(row) => `${row.day}-${row.user_id}-${row.kind}`}
        emptyContent={
          <EmptyState className='min-h-32' title={t('No earnings yet')} />
        }
        columns={[
          {
            id: 'day',
            header: t('Date'),
            cell: (row) => row.day.split('+')[0],
          },
          { id: 'user', header: t('Customer ID'), cell: (row) => row.user_id },
          {
            id: 'kind',
            header: t('Type'),
            cell: (row) => (
              <span>
                {kindLabel(t, row.kind)}
                {row.reason && (
                  <span className='text-muted-foreground ml-1 text-xs'>
                    ({reasonLabel(t, row.reason)})
                  </span>
                )}
              </span>
            ),
          },
          {
            id: 'revenue',
            header: t('Customer spend'),
            cell: (row) => amountCell(row.revenue_usd),
          },
          {
            id: 'earning',
            header: t('Your earnings'),
            cell: (row) => amountCell(row.earning_usd),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <StatusBadge
                label={statusLabel(t, row.status)}
                variant={row.status === 'available' ? 'success' : 'neutral'}
              />
            ),
          },
          {
            id: 'settled',
            header: t('Settled at'),
            cell: (row) =>
              row.settled_at > 0 ? formatTimestamp(row.settled_at) : '—',
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.page.total ?? 0}
        pageSize={OWNER_PAGE_SIZE}
        onPageChange={setPage}
      />
      <div className='flex flex-col gap-2'>
        <h2 className='text-sm font-medium'>{t('Request earnings')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Each request shows the model and billing group. Upstream channel names are not shown.'
          )}
        </p>
        <StaticDataTable
          tableClassName='min-w-[640px]'
          tableProps={{ 'aria-label': t('Request earnings') }}
          data={lines.data ?? []}
          getRowKey={(row) => row.id}
          emptyContent={
            <EmptyState className='min-h-24' title={t('No earnings yet')} />
          }
          columns={[
            {
              id: 'time',
              header: t('Time'),
              cell: (row) =>
                row.created_at > 0 ? formatTimestamp(row.created_at) : '—',
            },
            { id: 'model', header: t('Model'), cell: (row) => row.model_name },
            { id: 'group', header: t('Group'), cell: (row) => row.group },
            {
              id: 'request',
              header: t('Request'),
              cell: (row) => row.request_id || '—',
            },
            {
              id: 'earning',
              header: t('Your earnings'),
              cell: (row) => amountCell(row.earning_usd),
            },
          ]}
        />
      </div>
    </div>
  )
}
