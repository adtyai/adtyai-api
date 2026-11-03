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
import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { getUptimeStatus } from '@/features/dashboard/api'
import { useStatus } from '@/hooks/use-status'
import { toIntlLocale } from '@/i18n/languages'
import { ADTY_BRAND_NAME } from '@/lib/adty-brand'
import { cn } from '@/lib/utils'

import {
  isAllOperational,
  toPublicStatusGroups,
  type MonitorState,
  type PublicStatusGroup,
} from './lib/status'

const STATE_DOT: Record<MonitorState, string> = {
  up: 'bg-success',
  down: 'bg-destructive',
  pending: 'bg-warning',
  maintenance: 'bg-info',
  unknown: 'bg-muted-foreground/40',
}

function StateLabel(props: { state: MonitorState }) {
  const { t } = useTranslation()
  // Literal t() calls so the i18n scanner sees every label.
  switch (props.state) {
    case 'up':
      return <>{t('Operational')}</>
    case 'down':
      return <>{t('Down')}</>
    case 'pending':
      return <>{t('Degraded')}</>
    case 'maintenance':
      return <>{t('Maintenance')}</>
    default:
      return <>{t('Unknown')}</>
  }
}

function StatusGroup(props: { group: PublicStatusGroup; locale?: string }) {
  const headingId = `status-group-${props.group.id}`
  const percent = new Intl.NumberFormat(props.locale, {
    style: 'percent',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
  return (
    <section aria-labelledby={headingId} className='rounded-xl border'>
      <h3
        id={headingId}
        className='bg-muted/40 border-b px-4 py-2.5 text-sm font-semibold'
      >
        {props.group.name}
      </h3>
      <ul>
        {props.group.monitors.map((monitor) => (
          <li
            key={monitor.name}
            className='flex items-center justify-between gap-3 border-b px-4 py-3 last:border-b-0'
          >
            <span className='flex min-w-0 items-center gap-2.5'>
              <span
                aria-hidden
                className={cn(
                  'size-2 shrink-0 rounded-full',
                  STATE_DOT[monitor.state]
                )}
              />
              <span className='truncate text-sm'>{monitor.name}</span>
            </span>
            <span className='flex shrink-0 items-center gap-4 text-sm'>
              <span className='text-muted-foreground'>
                <StateLabel state={monitor.state} />
              </span>
              <span className='w-20 text-right font-mono tabular-nums'>
                {percent.format(monitor.uptime)}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

/**
 * Public service status (adty task 11.10): model and service availability
 * from the Uptime Kuma settings, without channel names. Anyone can open it;
 * the title uses the site name from /api/status, which task 11.2 will answer
 * per site, so partner sites show their own brand.
 */
export function StatusPage() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const { status, loading: statusLoading } = useStatus()
  const siteName =
    typeof status?.system_name === 'string' && status.system_name.trim()
      ? status.system_name.trim()
      : ADTY_BRAND_NAME
  // Same switch as the console uptime panel (use-status-data.ts).
  const enabled = Boolean(status) && status?.uptime_kuma_enabled !== false

  const uptime = useQuery({
    queryKey: ['adty', 'public-status'],
    queryFn: async () => toPublicStatusGroups((await getUptimeStatus()).data),
    enabled,
    refetchInterval: 60 * 1000,
    meta: { errorToast: false },
  })

  let content
  if (statusLoading || (enabled && uptime.isPending)) {
    content = <LoadingState />
  } else if (enabled && uptime.isError) {
    content = (
      <ErrorState
        title={t('Service status could not be loaded.')}
        onRetry={() => void uptime.refetch()}
      />
    )
  } else if (!enabled || !uptime.data?.length) {
    content = <EmptyState title={t('Service status is not available yet.')} />
  } else {
    const allUp = isAllOperational(uptime.data)
    content = (
      <div className='space-y-6'>
        <h2
          className={cn(
            'rounded-xl border px-4 py-3 text-base font-semibold',
            allUp ? 'bg-success-soft' : 'bg-destructive-soft'
          )}
        >
          {allUp ? t('All systems operational') : t('Some services are down')}
        </h2>
        {uptime.data.map((group) => (
          <StatusGroup key={group.id} group={group} locale={locale} />
        ))}
        <p className='text-muted-foreground text-xs'>
          {t('Uptime over the last 24 hours. Updated every minute.')}
        </p>
      </div>
    )
  }

  return (
    <PublicLayout>
      <div className='mx-auto max-w-3xl space-y-6 py-6'>
        <h1 className='text-2xl font-semibold tracking-tight'>
          {t('{{site}} status', { site: siteName })}
        </h1>
        {content}
      </div>
    </PublicLayout>
  )
}
