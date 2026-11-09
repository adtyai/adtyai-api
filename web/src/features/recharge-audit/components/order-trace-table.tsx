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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestamp } from '@/lib/format'

import type { OrderTraceEntry } from '../api'
import { ORDER_TRACE_LABELS } from '../trace-labels'

export function OrderTraceTable(props: { entries: OrderTraceEntry[] }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <StaticDataTable
      data={props.entries}
      getRowKey={(entry) => entry.id}
      tableProps={{ 'aria-label': t('Trace records') }}
      tableClassName='min-w-[900px]'
      emptyContent={
        <EmptyState title={t('No linked records')} className='min-h-32' />
      }
      columns={[
        {
          id: 'time',
          header: t('Time'),
          cell: (entry) => formatTimestamp(entry.created_at),
        },
        {
          id: 'type',
          header: t('Type'),
          cell: (entry) => (
            <div className='flex flex-col'>
              <span>{t(entry.kind)}</span>
              <span className='text-muted-foreground'>
                {entry.source &&
                  t(ORDER_TRACE_LABELS[entry.source] ?? entry.source)}{' '}
                ·{' '}
                {entry.status &&
                  t(ORDER_TRACE_LABELS[entry.status] ?? entry.status)}
              </span>
            </div>
          ),
        },
        {
          id: 'amount',
          header: t('Amount'),
          cell: (entry) => (
            <div className='flex flex-col'>
              <span>
                {entry.amount === undefined
                  ? '-'
                  : `${entry.amount} ${entry.currency || ''}`}
              </span>
              {entry.delta_quota !== undefined && (
                <span>
                  {t('Quota change')}: {formatNumber(entry.delta_quota, locale)}
                </span>
              )}
              {entry.balance_before !== undefined && (
                <span>
                  {entry.balance_before} → {entry.balance_after}
                </span>
              )}
              {entry.pending_delta !== undefined && (
                <span>
                  {t('Pending')}: {entry.pending_delta} · {t('Available')}:{' '}
                  {entry.available_delta} · {t('Frozen')}: {entry.frozen_delta}{' '}
                  · {t('Withdrawn')}: {entry.withdrawn_delta}
                </span>
              )}
            </div>
          ),
        },
        {
          id: 'reference',
          header: t('Reference'),
          cell: (entry) => (
            <div className='max-w-xs break-all'>
              <span>{entry.reference || `#${entry.id}`}</span>
              {entry.operator_id !== undefined && entry.operator_id > 0 && (
                <p>
                  {t('Operator')}: #{entry.operator_id}
                </p>
              )}
              {entry.note && (
                <p className='text-muted-foreground whitespace-pre-wrap'>
                  {entry.note}
                </p>
              )}
            </div>
          ),
        },
      ]}
    />
  )
}
