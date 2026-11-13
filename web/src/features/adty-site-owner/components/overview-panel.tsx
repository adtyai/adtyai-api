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
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { getOwnerEarnings, getOwnerStats, siteOwnerKeys } from '../api'
import { isNegativeAmount } from '../lib/site-owner'
import { OwnerStatGrid } from './stat-grid'

/** Overview: four earning numbers, a 30-day trend and the risk notice (task 11.6). */
export function OverviewPanel() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const stats = useQuery({
    queryKey: siteOwnerKeys.stats('', ''),
    queryFn: () => getOwnerStats('', ''),
  })
  const earnings = useQuery({
    queryKey: siteOwnerKeys.list('earnings', { p: 1, kind: '' }),
    queryFn: () => getOwnerEarnings(1, ''),
  })
  const balances = earnings.data?.balances
  const total = stats.data?.total
  const money = (value: string | undefined) =>
    formatUsd(Number(value ?? 0), locale)
  const chartConfig = {
    revenue: { label: t('Customer spend'), color: 'var(--chart-1)' },
    earning: { label: t('Your earnings'), color: 'var(--chart-2)' },
  } satisfies ChartConfig
  const series = (stats.data?.series ?? []).map((day) => ({
    day: day.day,
    revenue: Number(day.revenue_usd),
    earning: Number(day.earning_usd),
  }))
  const retry = () => {
    void stats.refetch()
    void earnings.refetch()
  }
  if ((stats.isError && !stats.data) || (earnings.isError && !earnings.data)) {
    return <ErrorState onRetry={retry} />
  }

  return (
    <div className='flex flex-col gap-4'>
      {(stats.isError || earnings.isError) && <ErrorState onRetry={retry} />}
      <OwnerStatGrid
        label={t('Earnings')}
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
          {
            label: t('Last 30 days'),
            value: money(total?.earning_usd),
            negative: isNegativeAmount(total?.earning_usd),
          },
        ]}
      />
      <OwnerStatGrid
        label={t('Last 30 days')}
        items={[
          {
            label: t('New customers'),
            value: formatNumber(total?.new_customers ?? 0, locale),
          },
          {
            label: t('Paying customers'),
            value: formatNumber(total?.paid_customers ?? 0, locale),
          },
          { label: t('Customer spend'), value: money(total?.revenue_usd) },
        ]}
      />
      {Number(balances?.skipped_usd ?? 0) !== 0 && (
        <Alert>
          <AlertDescription>
            {t(
              '{{amount}} of earnings is not counted because some customers are under review for self-use or moving from the main site.',
              { amount: money(balances?.skipped_usd) }
            )}
          </AlertDescription>
        </Alert>
      )}
      {series.length === 0 ? (
        <EmptyState className='min-h-48' title={t('No data yet')} />
      ) : (
        <ChartContainer config={chartConfig} className='h-64 w-full'>
          <AreaChart data={series} accessibilityLayer>
            <CartesianGrid vertical={false} />
            <XAxis dataKey='day' tickLine={false} axisLine={false} />
            <YAxis tickLine={false} axisLine={false} width={48} />
            <ChartTooltip content={<ChartTooltipContent />} />
            <Area
              dataKey='revenue'
              type='monotone'
              fill='var(--color-revenue)'
              stroke='var(--color-revenue)'
              fillOpacity={0.2}
            />
            <Area
              dataKey='earning'
              type='monotone'
              fill='var(--color-earning)'
              stroke='var(--color-earning)'
              fillOpacity={0.3}
            />
          </AreaChart>
        </ChartContainer>
      )}
    </div>
  )
}
