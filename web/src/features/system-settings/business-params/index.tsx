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
import { Info, Pencil, Search, ShieldAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

import { useAlertsConfig } from '../alerts/hooks/use-alerts'
import { isTelegramPendingConfigured } from '../alerts/lib/put'
import { EditParamDialog } from './components/edit-param-dialog'
import { useBusinessParams } from './hooks/use-business-params'
import {
  BUSINESS_PARAM_DESCRIPTIONS,
  formatDisplayValue,
  formatPageLabel,
  formatRange,
  formatSnapshot,
  groupRows,
  settingKeyLabel,
} from './lib/groups'
import type { BusinessParamRow } from './types'

const FIXED_RULES = [
  'Bonus wallet is deducted before paid quota',
  'Secrets show fingerprint only',
  'Refund / approve / callback are idempotent',
  'Balances never go negative',
  'With ≥2 admins, self-approve is forbidden',
  'Callbacks must verify signatures',
  'Health probe every minute; running heartbeat >30min = stuck',
] as const

export function BusinessParamsSettings() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useBusinessParams()
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<BusinessParamRow | null>(null)

  const grouped = useMemo(() => {
    const rows = data?.rows ?? []
    const q = query.trim().toLowerCase()
    const filtered = q
      ? rows.filter((r) => {
          const desc =
            BUSINESS_PARAM_DESCRIPTIONS[r.key] || r.description || r.notes || ''
          return (
            r.key.toLowerCase().includes(q) ||
            desc.toLowerCase().includes(q) ||
            t(desc).toLowerCase().includes(q) ||
            (r.page ?? '').toLowerCase().includes(q) ||
            formatPageLabel(r.page, t).toLowerCase().includes(q)
          )
        })
      : rows
    return groupRows(filtered)
  }, [data?.rows, query, t])

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Business parameters')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='relative w-56'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2' />
          <Input
            className='h-8 pl-8 text-sm'
            placeholder={t('Search parameter name')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Values are range-checked on save. Money params apply to new orders only. Saves require a second confirmation.'
            )}
            {data?.meta?.excluded?.length ? (
              <>
                {' '}
                {t('Excluded from this page')}:{' '}
                <span className='text-xs'>
                  {data.meta.excluded
                    .map((k) => t(settingKeyLabel(k)))
                    .join('、')}
                </span>
              </>
            ) : null}
          </p>

          <div className='grid gap-3 md:grid-cols-3'>
            <HintCard
              tone='brand'
              title={t('Value check')}
              body={t(
                'The system checks the value range on save and shows an error if it is out of range.'
              )}
            />
            <HintCard
              tone='amber'
              title={t('Snapshot effect')}
              body={t(
                'Money-related values are snapshotted into the order at create time. Changes only affect new orders; affiliate freeze days snapshot when the commission is created.'
              )}
            />
          </div>

          <DailyReportHint />

          {isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading settings...')}
            </div>
          ) : null}

          {isError && !data ? (
            <ErrorState
              title={t('Failed to load settings')}
              onRetry={() => void refetch()}
            />
          ) : null}

          {!isLoading && data
            ? grouped.map(({ group, rows }) => (
                <section
                  key={group.id + group.titleKey}
                  className='bg-card shrink-0 overflow-hidden rounded-xl border shadow-xs'
                >
                  <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                    <div>
                      <div className='text-[15px] font-semibold'>
                        {t(group.titleKey)}
                      </div>
                      <div className='text-muted-foreground mt-0.5 text-xs'>
                        {rows.length} {t('items')}
                      </div>
                    </div>
                  </div>
                  <Table>
                    <TableHeader>
                      <TableRow className='bg-muted/40 hover:bg-muted/40'>
                        <TableHead>{t('Parameter')}</TableHead>
                        <TableHead>{t('Current')}</TableHead>
                        <TableHead>{t('Default')}</TableHead>
                        <TableHead>{t('Range')}</TableHead>
                        <TableHead>{t('Description')}</TableHead>
                        <TableHead>{t('Page')}</TableHead>
                        <TableHead>{t('Effect')}</TableHead>
                        <TableHead className='w-[1%]' />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {rows.map((row) => {
                        const desc =
                          BUSINESS_PARAM_DESCRIPTIONS[row.key] ||
                          row.description ||
                          row.notes ||
                          ''
                        return (
                          <TableRow key={row.key}>
                            <TableCell className='min-w-[9rem] whitespace-normal'>
                              <span className='text-xs font-medium'>
                                {t(settingKeyLabel(row.key))}
                              </span>
                            </TableCell>
                            <TableCell>
                              <span className='font-semibold'>
                                {formatDisplayValue(row, t)}
                              </span>
                            </TableCell>
                            <TableCell className='max-w-[10rem] break-all whitespace-normal'>
                              <span
                                className='text-muted-foreground text-xs'
                                data-table-text='secondary'
                              >
                                {row.default || '—'}
                              </span>
                            </TableCell>
                            <TableCell className='min-w-[7rem] whitespace-normal'>
                              <code className='text-muted-foreground text-[11.5px]'>
                                {t(formatRange(row))}
                              </code>
                            </TableCell>
                            <TableCell className='min-w-[12rem] whitespace-normal'>
                              <span
                                className='text-muted-foreground text-xs'
                                data-table-text='secondary'
                              >
                                {t(desc) || desc || '—'}
                              </span>
                            </TableCell>
                            <TableCell>
                              {row.page ? (
                                <Badge variant='secondary'>
                                  {formatPageLabel(row.page, t)}
                                </Badge>
                              ) : (
                                '—'
                              )}
                            </TableCell>
                            <TableCell className='max-w-[13rem] min-w-[8rem] whitespace-normal'>
                              <span
                                className='text-xs'
                                data-table-text='secondary'
                              >
                                {t(formatSnapshot(row.snapshot))}
                              </span>
                            </TableCell>
                            <TableCell>
                              <div className='flex flex-col items-start gap-1.5'>
                                <Button
                                  type='button'
                                  variant='outline'
                                  size='sm'
                                  className='h-7 px-2.5 text-xs'
                                  onClick={() => setEditing(row)}
                                >
                                  <Pencil className='size-3.5' />
                                  {t('Edit')}
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </section>
              ))
            : null}

          <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
            <div className='border-b px-5 pt-4 pb-3'>
              <div className='text-[15px] font-semibold'>
                {t('Hard-coded rules (not configurable)')}
              </div>
              <div className='text-muted-foreground mt-0.5 text-xs'>
                {t('These rules are fixed in code and have no switches')}
              </div>
            </div>
            <ul className='grid gap-2 p-5 sm:grid-cols-2'>
              {FIXED_RULES.map((rule) => (
                <li
                  key={rule}
                  className='bg-muted/40 text-foreground flex items-center gap-2 rounded-lg border px-3 py-2 text-[13px]'
                >
                  <ShieldAlert className='text-muted-foreground size-3.5 shrink-0' />
                  {t(rule)}
                </li>
              ))}
            </ul>
          </section>
        </div>
      </SectionPageLayout.Content>

      <EditParamDialog
        row={editing}
        open={Boolean(editing)}
        onOpenChange={(open) => {
          if (!open) setEditing(null)
        }}
      />
    </SectionPageLayout>
  )
}

function DailyReportHint() {
  const { t } = useTranslation()
  const alertsQuery = useAlertsConfig()
  const configured = isTelegramPendingConfigured(alertsQuery.data)

  return (
    <div className='flex flex-wrap items-start gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
      <Info className='mt-0.5 size-4 shrink-0' />
      <div className='min-w-0 flex-1 space-y-1'>
        <div className='font-medium'>
          {t('Daily report push time (HH:mm, Asia/Shanghai)')}
        </div>
        <p>
          {t(
            'Edit DailyReportTime in Alerts & ops below. Timezone is Asia/Shanghai. Format HH:mm (e.g. 09:00).'
          )}
        </p>
        <p>
          {t('Daily report is sent via the Telegram pending alert channel.')}{' '}
          <Link
            to='/system-settings/alerts'
            className='font-medium underline underline-offset-2'
          >
            {t('Open Telegram alerts')}
          </Link>
        </p>
        <div className='flex flex-wrap items-center gap-2 pt-0.5'>
          {alertsQuery.isLoading ? (
            <span className='text-xs opacity-80'>
              {t('Loading settings...')}
            </span>
          ) : null}
          {!alertsQuery.isLoading && configured ? (
            <Badge variant='secondary'>
              {t('Telegram pending channel configured')}
            </Badge>
          ) : null}
          {!alertsQuery.isLoading && !configured ? (
            <Badge variant='outline'>
              {t('Telegram pending channel not configured')}
            </Badge>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function HintCard({
  title,
  body,
  tone,
}: {
  title: string
  body: string
  tone: 'brand' | 'amber' | 'slate'
}) {
  return (
    <div
      className={cn(
        'flex gap-2.5 rounded-lg border px-3.5 py-2.5 text-[13px] leading-5',
        tone === 'brand' &&
          'border-indigo-100 bg-indigo-50/70 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100',
        tone === 'amber' &&
          'border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100',
        tone === 'slate' && 'border-border bg-muted/40 text-foreground'
      )}
    >
      <Info className='mt-0.5 size-4 shrink-0' />
      <div>
        <div className='mb-0.5 font-medium'>{title}</div>
        {body}
      </div>
    </div>
  )
}

export default BusinessParamsSettings
