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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  getSignupSources,
  marketingKeys,
  type SignupSourceDimension,
} from '../api'
import { dateInputToUnix } from '../lib/campaign'

const DIMENSIONS: SignupSourceDimension[] = [
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'referrer_host',
  'landing_path',
  'inviter',
  'site',
]

/** Sign-ups, paying users, top-ups and spend per source (adty task 12.5). */
export function SourcesPanel() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [dimension, setDimension] =
    useState<SignupSourceDimension>('utm_source')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const params = {
    dimension,
    from: dateInputToUnix(from),
    // The end date is inclusive: count sign-ups until the next midnight.
    to: to ? dateInputToUnix(to) + 86400 : 0,
  }
  const query = useQuery({
    queryKey: marketingKeys.sources(params),
    queryFn: () => getSignupSources(params),
  })
  const labels: Record<SignupSourceDimension, string> = {
    utm_source: t('UTM source'),
    utm_medium: t('UTM medium'),
    utm_campaign: t('UTM campaign'),
    referrer_host: t('Referring site'),
    landing_path: t('Landing page'),
    inviter: t('Referrer user ID'),
    site: t('Site ID'),
  }

  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }

  return (
    <div className='flex flex-col gap-3'>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-2'>
          <Label htmlFor='sources-dimension'>{t('Group by')}</Label>
          <NativeSelect
            id='sources-dimension'
            value={dimension}
            onChange={(event) =>
              setDimension(event.target.value as SignupSourceDimension)
            }
          >
            {DIMENSIONS.map((item) => (
              <NativeSelectOption key={item} value={item}>
                {labels[item]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        <div className='space-y-2'>
          <Label htmlFor='sources-from'>{t('Signed up on or after')}</Label>
          <Input
            id='sources-from'
            type='date'
            value={from}
            onChange={(event) => setFrom(event.target.value)}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='sources-to'>{t('Signed up on or before')}</Label>
          <Input
            id='sources-to'
            type='date'
            value={to}
            onChange={(event) => setTo(event.target.value)}
          />
        </div>
      </div>
      <StaticDataTable
        tableClassName='min-w-[720px]'
        tableProps={{ 'aria-label': t('Sign-up sources') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => row.key}
        emptyContent={
          <EmptyState
            className='min-h-32'
            title={t('No sign-ups in this period')}
          />
        }
        columns={[
          {
            id: 'key',
            header: labels[dimension],
            cell: (row) =>
              row.key && row.key !== '0' ? row.key : t('Direct / none'),
          },
          {
            id: 'signups',
            header: t('Sign-ups'),
            cell: (row) => formatNumber(row.signups, locale),
          },
          {
            id: 'paying',
            header: t('Paying users'),
            cell: (row) => formatNumber(row.paying, locale),
          },
          {
            id: 'topup',
            header: t('Top-ups (USD)'),
            cell: (row) => formatUsd(Number(row.topup_usd), locale),
          },
          {
            id: 'consume',
            header: t('Spend (USD)'),
            cell: (row) => formatUsd(Number(row.consume_usd), locale),
          },
        ]}
      />
    </div>
  )
}
