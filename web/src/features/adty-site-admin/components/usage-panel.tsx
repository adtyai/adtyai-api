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

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'

import { getResellerUsage, siteAdminKeys } from '../api'

/** Resend sender domain slots and Turnstile widget slots (tasks 11.3, 11.8). Read-only. */
export function UsagePanel() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const query = useQuery({
    queryKey: siteAdminKeys.list('usage', {}),
    queryFn: getResellerUsage,
  })
  const usage = query.data
  if (!usage) return null

  return (
    <div className='grid max-w-3xl gap-4 sm:grid-cols-2'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Sender domains (Resend)')}</CardTitle>
        </CardHeader>
        <CardContent className='grid gap-2 text-sm'>
          <dl className='grid grid-cols-2 gap-1'>
            <dt className='text-muted-foreground'>{t('Using a slot')}</dt>
            <dd>{formatNumber(usage.sender.in_use, locale)}</dd>
            <dt className='text-muted-foreground'>{t('Verified')}</dt>
            <dd>{formatNumber(usage.sender.verified, locale)}</dd>
            <dt className='text-muted-foreground'>
              {t('Verification failed')}
            </dt>
            <dd>{formatNumber(usage.sender.failed, locale)}</dd>
          </dl>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Each domain uses one slot of the Resend plan (Pro 10, Scale 1,000) until it is deleted.'
            )}
          </p>
          {usage.sender.quota_full_at > 0 && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t(
                  'The last sender domain request was refused because no Resend slots were left ({{time}}).',
                  { time: formatTimestampToDate(usage.sender.quota_full_at) }
                )}
              </AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('Turnstile widgets')}</CardTitle>
        </CardHeader>
        <CardContent className='grid gap-2 text-sm'>
          {usage.widget_slots_low && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t(
                  'Only {{count}} Turnstile domain slots are left. Add another widget before they run out.',
                  { count: usage.widget_slots_left }
                )}
              </AlertDescription>
            </Alert>
          )}
          <StaticDataTable
            tableProps={{ 'aria-label': t('Turnstile widgets') }}
            data={usage.widgets}
            getRowKey={(row) => row.id}
            emptyContent={
              <EmptyState
                className='min-h-16'
                title={t('No widgets configured')}
              />
            }
            columns={[
              {
                id: 'id',
                header: t('Widget'),
                cell: (row) => (
                  <span className='font-mono text-xs'>{row.id}</span>
                ),
              },
              {
                id: 'used',
                header: t('Domains'),
                cell: (row) => `${row.used} / ${row.limit}`,
              },
            ]}
          />
          {usage.unassigned_domains > 0 && (
            <p className='text-muted-foreground text-xs'>
              {t(
                '{{count}} custom domains have no widget; sign-up and sign-in on them are verified on the platform subdomain.',
                { count: usage.unassigned_domains }
              )}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
