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

import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  HandCoins,
  Info,
  RefreshCw,
  Wallet,
  Zap,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { useAdminOverview } from './hooks/use-overview'
import {
  formatOverviewAsOf,
  formatOverviewCount,
  formatOverviewDateShort,
  formatOverviewQuota,
  formatOverviewUsd,
} from './lib/format'
import type { OverviewDayStats } from './types'

function DayStatsCard({
  title,
  day,
  accent,
}: {
  title: string
  day: OverviewDayStats
  accent?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'rounded-xl border border-border/80 bg-card p-5 shadow-[0_1px_2px_rgba(16,24,40,.04)]',
        accent && 'border-indigo-100 dark:border-indigo-900/40'
      )}
    >
      <div className='mb-4 flex items-baseline justify-between gap-2'>
        <h2 className='text-[15px] font-semibold text-slate-900 dark:text-slate-50'>
          {title}
        </h2>
        <span className='text-muted-foreground font-mono text-xs'>
          {formatOverviewDateShort(day.date)}
        </span>
      </div>

      <div className='grid gap-4 sm:grid-cols-2'>
        <div
          className={cn(
            'rounded-xl p-4',
            accent
              ? 'from-primary to-violet-600 text-primary-foreground bg-gradient-to-br'
              : 'bg-muted/40 border border-border/60'
          )}
        >
          <div className='flex items-center justify-between gap-2'>
            <span
              className={cn(
                'text-[13px]',
                accent ? 'text-white/80' : 'text-muted-foreground'
              )}
            >
              {t('Overview topup paid')}
            </span>
            <span
              className={cn(
                'grid size-8 place-items-center rounded-lg',
                accent
                  ? 'bg-white/15'
                  : 'bg-muted text-muted-foreground border border-border/60'
              )}
            >
              <Wallet className='size-4' aria-hidden='true' />
            </span>
          </div>
          <div
            className={cn(
              'mt-2 text-[24px] font-semibold tracking-tight',
              !accent && 'text-slate-900 dark:text-slate-50'
            )}
          >
            {formatOverviewUsd(day.topup_paid_usd)}
          </div>
          <div
            className={cn(
              'mt-1 text-xs',
              accent ? 'text-white/80' : 'text-muted-foreground'
            )}
          >
            {t('{{count}} paid orders', {
              count: formatOverviewCount(day.topup_paid_orders),
            })}
          </div>
        </div>

        <div className='bg-muted/40 border-border/60 rounded-xl border p-4'>
          <div className='flex items-center justify-between gap-2'>
            <span className='text-muted-foreground text-[13px]'>
              {t('Overview consume quota')}
            </span>
            <span className='bg-muted text-muted-foreground border-border/60 grid size-8 place-items-center rounded-lg border'>
              <Zap className='size-4' aria-hidden='true' />
            </span>
          </div>
          <div className='mt-2 text-[24px] font-semibold tracking-tight text-slate-900 dark:text-slate-50'>
            {formatOverviewQuota(day.consume_quota)}
          </div>
          <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs'>
            <span>
              {t('Overview consume paid')}:{' '}
              <span className='font-mono'>
                {formatOverviewQuota(day.consume_quota_paid)}
              </span>
            </span>
            <span>
              {t('Overview consume bonus')}:{' '}
              <span className='font-mono'>
                {formatOverviewQuota(day.consume_quota_bonus)}
              </span>
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}

export function OverviewSettings() {
  const { t } = useTranslation()
  const overview = useAdminOverview()
  const isForbidden =
    (overview.error as { status?: number } | null)?.status === 403
  const data = overview.data

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Operations overview')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void overview.refetch()}
          disabled={overview.isFetching}
        >
          <RefreshCw
            className={cn('size-3.5', overview.isFetching && 'animate-spin')}
          />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Admin operations overview for today and yesterday (Asia/Shanghai). Top-up paid USD, consume quota split by paid/bonus, and pending affiliate withdrawals. No gross margin.'
            )}
          </p>

          <div className='grid gap-3'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Overview shows no gross margin or cost. Timezone is Asia/Shanghai.'
                )}
              </div>
            </div>
          </div>

          {overview.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading settings...')}
            </div>
          ) : null}

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'Admin permission is required to view operations overview.'
              )}
            />
          ) : null}

          {overview.isError && !overview.data && !isForbidden ? (
            <ErrorState
              title={t('Failed to load operations overview')}
              onRetry={() => void overview.refetch()}
            />
          ) : null}

          {data ? (
            <div className='flex flex-col gap-4'>
              <p className='text-muted-foreground text-xs'>
                {t('Data as of {{time}} ({{timezone}})', {
                  time: formatOverviewAsOf(
                    data.as_of,
                    data.timezone || 'Asia/Shanghai'
                  ),
                  timezone:
                    (data.timezone || 'Asia/Shanghai') === 'Asia/Shanghai'
                      ? t('Beijing time')
                      : data.timezone,
                })}
              </p>

              <div className='grid gap-4 lg:grid-cols-2'>
                <DayStatsCard
                  title={t('Overview today')}
                  day={data.today}
                  accent
                />
                <DayStatsCard
                  title={t('Overview yesterday')}
                  day={data.yesterday}
                />
              </div>

              <div className='border-border/80 bg-card rounded-xl border p-5 shadow-[0_1px_2px_rgba(16,24,40,.04)]'>
                <div className='flex flex-wrap items-center justify-between gap-3'>
                  <div className='flex items-center gap-3'>
                    <span className='bg-muted text-muted-foreground border-border/60 grid size-10 place-items-center rounded-lg border'>
                      <HandCoins className='size-5' aria-hidden='true' />
                    </span>
                    <div>
                      <div className='text-[15px] font-semibold text-slate-900 dark:text-slate-50'>
                        {t('Overview pending withdrawals')}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {t(
                          'Pending, approved, pay_failed, and pay_unknown affiliate withdrawals'
                        )}
                      </div>
                    </div>
                  </div>
                  <div className='flex items-center gap-3'>
                    <span className='text-2xl font-semibold text-slate-900 tabular-nums dark:text-slate-50'>
                      {formatOverviewCount(data.pending_withdrawals)}
                    </span>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-8'
                      render={<Link to='/system-settings/aff-withdrawals' />}
                    >
                      {t('Go to affiliate withdrawals')}
                      <ArrowRight className='size-3.5' />
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
