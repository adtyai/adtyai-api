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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatTimestamp } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  siteWalletKindLabel,
  siteWalletLedgerTypeLabel,
  type SiteWalletKind,
  type SiteWalletLedgerEntry,
  type SiteWalletLedgerPage,
} from './site-wallet-labels'

const PAGE_SIZE = 20

/** Exact decimal from the server with an explicit sign; zero shows as a dash. */
function signedUsd(value: string) {
  const amount = Number(value)
  if (!value || amount === 0) return '—'
  return `${amount > 0 ? '+' : ''}${value} USD`
}

/**
 * Site wallet ledger, newest first, one page at a time. Root passes every
 * wallet kind; the site owner passes earnings and margin only, because the
 * operating wallet is never shown to owners.
 */
export function SiteWalletLedger(props: {
  label: string
  queryKey: readonly unknown[]
  kinds: SiteWalletKind[]
  fetchPage: (
    kind: string,
    page: number,
    pageSize: number
  ) => Promise<SiteWalletLedgerPage>
  showOperator?: boolean
}) {
  const { t } = useTranslation()
  const [kind, setKind] = useState('')
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: [...props.queryKey, kind, page],
    queryFn: () => props.fetchPage(kind, page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  // A success response with an unexpected shape shows an empty page instead of crashing.
  const rows = Array.isArray(query.data?.items) ? query.data.items : []
  const total = Number(query.data?.total) || 0
  const amountCell = (value: string) => (
    <span
      className={cn('tabular-nums', Number(value) < 0 && 'text-destructive')}
    >
      {signedUsd(value)}
    </span>
  )
  const columns = [
    {
      id: 'time',
      header: t('Time'),
      cell: (row: SiteWalletLedgerEntry) => formatTimestamp(row.created_at),
    },
    {
      id: 'wallet',
      header: t('Wallet'),
      cell: (row: SiteWalletLedgerEntry) =>
        siteWalletKindLabel(t, row.wallet_kind),
    },
    {
      id: 'type',
      header: t('Type'),
      cell: (row: SiteWalletLedgerEntry) =>
        siteWalletLedgerTypeLabel(t, row.type),
    },
    {
      id: 'delta',
      header: t('Change'),
      cell: (row: SiteWalletLedgerEntry) => amountCell(row.delta),
    },
    {
      id: 'hold',
      header: t('Frozen change'),
      cell: (row: SiteWalletLedgerEntry) => amountCell(row.hold_delta),
    },
    {
      id: 'balance',
      header: t('Balance'),
      cell: (row: SiteWalletLedgerEntry) => (
        <span className='tabular-nums'>
          {row.balance_before} → {row.balance_after}
        </span>
      ),
    },
    {
      id: 'reference',
      header: t('Reference'),
      cell: (row: SiteWalletLedgerEntry) =>
        row.ref_id ? (
          <code className='font-mono text-xs break-all'>{row.ref_id}</code>
        ) : (
          '—'
        ),
    },
    ...(props.showOperator
      ? [
          {
            id: 'operator',
            header: t('Operator'),
            cell: (row: SiteWalletLedgerEntry) =>
              row.operator_id > 0 ? `#${row.operator_id}` : '—',
          },
        ]
      : []),
  ]

  return (
    <section aria-label={props.label} className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{props.label}</h3>
        <NativeSelect
          aria-label={t('Wallet')}
          value={kind}
          onChange={(event) => {
            setKind(event.target.value)
            setPage(1)
          }}
        >
          <NativeSelectOption value=''>{t('All wallets')}</NativeSelectOption>
          {props.kinds.map((value) => (
            <NativeSelectOption key={value} value={value}>
              {siteWalletKindLabel(t, value)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isPending ? (
        <LoadingState />
      ) : (
        query.data && (
          <>
            <StaticDataTable
              tableClassName='min-w-[860px]'
              tableProps={{ 'aria-label': props.label }}
              data={rows}
              getRowKey={(row) => String(row.id)}
              emptyContent={
                <EmptyState className='min-h-24' title={t('No records')} />
              }
              columns={columns}
            />
            <PageSwitcher
              page={page}
              total={total}
              pageSize={PAGE_SIZE}
              onPageChange={setPage}
            />
          </>
        )
      )}
    </section>
  )
}
