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

import { Info, RefreshCw, Search } from 'lucide-react'
import { useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { useAdminBonusLedger } from './hooks/use-bonus-ledger'
import {
  DEFAULT_LEDGER_LIMIT,
  LEDGER_KIND_OPTIONS,
  datetimeLocalToUnix,
  formatLedgerTime,
  formatSignedQuota,
  ledgerKindLabelKey,
  ledgerNote,
  parseUserIdFilter,
} from './lib/bonus-ledger'
import type { AdminBonusLedgerParams } from './types'

export function BonusLedgerSettings() {
  const { t, i18n } = useTranslation()
  const [userIdDraft, setUserIdDraft] = useState('')
  const [kindDraft, setKindDraft] = useState('all')
  const [fromDraft, setFromDraft] = useState('')
  const [toDraft, setToDraft] = useState('')
  const [page, setPage] = useState(0)
  const [applied, setApplied] = useState<{
    user_id?: number
    kind: string
    created_from?: number
    created_to?: number
  }>({ kind: '' })

  const listParams: AdminBonusLedgerParams = useMemo(
    () => ({
      user_id: applied.user_id,
      kind: applied.kind || undefined,
      created_from: applied.created_from,
      created_to: applied.created_to,
      limit: DEFAULT_LEDGER_LIMIT,
      offset: page * DEFAULT_LEDGER_LIMIT,
    }),
    [applied, page]
  )

  const list = useAdminBonusLedger(listParams)
  const rows = list.data?.rows ?? []
  const total = list.data?.total ?? 0
  const isForbidden = (list.error as { status?: number } | null)?.status === 403

  function onSearch(e?: FormEvent) {
    e?.preventDefault()
    setPage(0)
    setApplied({
      user_id: parseUserIdFilter(userIdDraft),
      kind: kindDraft === 'all' ? '' : kindDraft,
      created_from: datetimeLocalToUnix(fromDraft),
      created_to: datetimeLocalToUnix(toDraft),
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Bonus ledger')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void list.refetch()}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Admin bonus wallet ledger (account=bonus only). Bonus never expires.'
            )}
          </p>

          <div className='grid gap-3'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Bonus never expires. This list is forced to account=bonus; paid top-up ledger is not shown here.'
                )}
              </div>
            </div>
          </div>

          <form className='flex flex-wrap items-end gap-2' onSubmit={onSearch}>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>{t('User')}</span>
              <Input
                value={userIdDraft}
                onChange={(e) => setUserIdDraft(e.target.value)}
                placeholder={t('User ID')}
                className='h-9 w-28 font-mono text-sm'
                inputMode='numeric'
              />
            </div>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Ledger kind filter')}
              </span>
              <Select
                items={[
                  { value: 'all', label: t('All kinds') },
                  ...LEDGER_KIND_OPTIONS.map((k) => ({
                    value: k,
                    label: t(ledgerKindLabelKey(k)),
                  })),
                ]}
                value={kindDraft}
                onValueChange={(v) => {
                  if (v) setKindDraft(v)
                }}
              >
                <SelectTrigger className='h-9 w-44 text-sm'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='all'>{t('All kinds')}</SelectItem>
                  {LEDGER_KIND_OPTIONS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(ledgerKindLabelKey(k))}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Created from')}
              </span>
              <Input
                type='datetime-local'
                value={fromDraft}
                onChange={(e) => setFromDraft(e.target.value)}
                className='h-9 w-48 text-sm'
              />
            </div>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Created to')}
              </span>
              <Input
                type='datetime-local'
                value={toDraft}
                onChange={(e) => setToDraft(e.target.value)}
                className='h-9 w-48 text-sm'
              />
            </div>
            <Button type='submit' size='sm' className='h-9'>
              <Search className='size-3.5' />
              {t('Search')}
            </Button>
          </form>

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'Admin permission is required to view bonus ledger.'
              )}
            />
          ) : null}

          {list.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}

          {list.isError && !isForbidden && !list.data ? (
            <ErrorState
              title={t('Failed to load bonus ledger')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
              <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                <div>
                  <div className='text-[15px] font-semibold'>
                    {t('Bonus ledger')}
                  </div>
                  <div className='text-muted-foreground mt-0.5 text-xs'>
                    {t('Total {{count}} records', { count: total })}
                  </div>
                </div>
                <div className='text-muted-foreground flex items-center gap-2 text-xs'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-7 px-2 text-xs'
                    disabled={page <= 0}
                    onClick={() => setPage((p) => Math.max(0, p - 1))}
                  >
                    {t('Previous page')}
                  </Button>
                  <span>{t('Page {{n}}', { n: page + 1 })}</span>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-7 px-2 text-xs'
                    disabled={(page + 1) * DEFAULT_LEDGER_LIMIT >= total}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    {t('Next page')}
                  </Button>
                </div>
              </div>
              <Table>
                <TableHeader>
                  <TableRow className='bg-muted/40 hover:bg-muted/40'>
                    <TableHead>{t('Ledger time')}</TableHead>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Ledger kind filter')}</TableHead>
                    <TableHead>{t('Ledger delta')}</TableHead>
                    <TableHead>{t('Balance after')}</TableHead>
                    <TableHead>{t('Ledger note')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={6}
                        className='text-muted-foreground h-24 text-center text-sm'
                      >
                        {t('No ledger entries')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const deltaCls =
                        row.delta > 0
                          ? 'text-amber-600 font-medium'
                          : row.delta < 0
                            ? 'text-slate-800 font-medium'
                            : 'text-muted-foreground'
                      return (
                        <TableRow key={`${row.id}-${row.created_at}`}>
                          <TableCell>
                            <span className='text-muted-foreground text-xs whitespace-nowrap'>
                              {formatLedgerTime(row.created_at, i18n.language)}
                            </span>
                          </TableCell>
                          <TableCell className='font-mono text-sm'>
                            {row.user_id}
                          </TableCell>
                          <TableCell>
                            <Badge variant='outline' className='font-normal'>
                              {t(ledgerKindLabelKey(row.kind))}
                            </Badge>
                          </TableCell>
                          <TableCell className={cn('tabular-nums', deltaCls)}>
                            {formatSignedQuota(row.delta)}
                          </TableCell>
                          <TableCell className='font-semibold tabular-nums'>
                            {formatQuota(row.balance_after)}
                          </TableCell>
                          <TableCell>
                            <span className='text-muted-foreground line-clamp-2 max-w-xs text-xs'>
                              {ledgerNote(row, t)}
                            </span>
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </section>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
