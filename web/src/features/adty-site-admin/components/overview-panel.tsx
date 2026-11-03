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
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'

import { getSitesOverview, siteAdminKeys } from '../api'

function Stat(props: { label: string; value: string }) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='text-lg font-medium'>{props.value}</div>
    </div>
  )
}

/** Totals for every reseller site, including hostname cost and settlement holds. */
export function OverviewPanel() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: siteAdminKeys.list('overview', {}),
    queryFn: getSitesOverview,
  })
  if (query.isLoading) return <LoadingState />
  const data = query.data
  if (!data) {
    return <EmptyState className='min-h-48' title={t('No data')} />
  }
  return (
    <div className='flex flex-col gap-4'>
      <div className='grid gap-3 sm:grid-cols-3'>
        <Stat label={t('Revenue')} value={data.site_revenue_usd} />
        <Stat label={t('Earnings')} value={data.site_earning_usd} />
        <Stat label={t('Top-up')} value={data.site_topup_usd} />
        <Stat
          label={t('Custom hostnames')}
          value={String(data.custom_hostnames)}
        />
        <Stat
          label={t('Hostname monthly fee')}
          value={data.hostname_monthly_fee_usd}
        />
        <Stat
          label={t('Earning hook failures')}
          value={String(data.earning_hook_failures)}
        />
      </div>
      <section className='flex flex-col gap-2'>
        <h3 className='text-sm font-medium'>{t('Settlement holds')}</h3>
        {data.settlement_holds.length === 0 ? (
          <EmptyState className='min-h-24' title={t('No data')} />
        ) : (
          <ul>
            {data.settlement_holds.map((hold) => (
              <li key={hold.site_id}>
                {hold.subdomain}: {hold.settle_hold_reason}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
