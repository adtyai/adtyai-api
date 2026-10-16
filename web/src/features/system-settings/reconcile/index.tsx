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

import { Info, Search } from 'lucide-react'
import { useState, type ReactNode } from 'react'
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

import { useReconcile } from './hooks/use-reconcile'
import {
  defaultReconcileDate,
  formatLastBackup,
  formatMissingBuckets,
  backfillStatusLabelKey,
  formatAccountDriftUsers,
  formatQuota,
  hasMissingBuckets,
  reconcileStatusLabelKey,
  reconcileStatusVariant,
  showFailedNote,
  showIncompleteOnly,
  showMismatchDetails,
  showOkSummary,
} from './lib/format'
import type { ReconcileResult } from './types'

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <Badge variant={reconcileStatusVariant(status)}>
      {t(reconcileStatusLabelKey(status))}
    </Badge>
  )
}

function MetaRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className='flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm'>
      <span className='text-muted-foreground min-w-28'>{label}</span>
      <span className='font-mono text-xs'>{value}</span>
    </div>
  )
}

function AccountDriftRow({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  const value = formatAccountDriftUsers(data)
  return (
    <MetaRow
      label={t('Account split drift users')}
      value={
        <span>
          {value ?? t('Not enabled')}{' '}
          <span className='text-muted-foreground font-sans'>
            {t('(report only, not counted toward balance)')}
          </span>
        </span>
      }
    />
  )
}

function IncompletePanel({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  const buckets = formatMissingBuckets(data.missing_buckets)
  return (
    <div className='space-y-3 rounded-xl border border-amber-200 bg-amber-50/80 p-5 shadow-xs dark:border-amber-900/50 dark:bg-amber-950/30'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge status={data.status} />
        <span className='text-sm font-medium text-amber-950 dark:text-amber-100'>
          {t('Hourly buckets incomplete — full reconcile result is withheld')}
        </span>
      </div>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Do not treat this as a complete daily reconcile. Fix or wait for missing hour buckets, then query again.'
        )}
      </p>
      <MetaRow
        label={t('Missing buckets')}
        value={
          hasMissingBuckets(data.missing_buckets) ? (
            <span className='break-all'>{buckets}</span>
          ) : (
            t('No missing buckets listed')
          )
        }
      />
      <MetaRow
        label={t('Backfill status')}
        value={t(backfillStatusLabelKey(data.backfill_status))}
      />
    </div>
  )
}

function FailedPanel({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  return (
    <div className='space-y-3 rounded-xl border border-red-200 bg-red-50/80 p-5 shadow-xs dark:border-red-900/50 dark:bg-red-950/30'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge status={data.status} />
        <span className='text-sm font-medium text-red-950 dark:text-red-100'>
          {t('Reconcile run failed')}
        </span>
      </div>
      <p className='font-mono text-xs break-all whitespace-pre-wrap'>
        {data.note || t('No error note from server')}
      </p>
    </div>
  )
}

function RefundTable({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  const r = data.refund
  return (
    <div className='overflow-hidden rounded-lg border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Refund check')}</TableHead>
            <TableHead className='text-right'>{t('Value')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>{t('Registered quota')}</TableCell>
            <TableCell className='text-right font-mono text-xs'>
              {formatQuota(r?.registered_quota)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>{t('Paid clawed')}</TableCell>
            <TableCell className='text-right font-mono text-xs'>
              {formatQuota(r?.paid_clawed)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>{t('Registered equals paid clawed')}</TableCell>
            <TableCell className='text-right'>
              <Badge variant={r?.equal ? 'secondary' : 'destructive'}>
                {r?.equal ? t('Equal') : t('Not equal')}
              </Badge>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>{t('Bonus clawed')}</TableCell>
            <TableCell className='text-right font-mono text-xs'>
              {formatQuota(r?.bonus_clawed)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>{t('Affiliate clawed')}</TableCell>
            <TableCell className='text-right font-mono text-xs'>
              {formatQuota(r?.aff_clawed)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              {t('Bonus shortfall')}
              <span className='text-muted-foreground ml-2 text-xs'>
                ({t('listed only; does not count as imbalance')})
              </span>
            </TableCell>
            <TableCell className='text-right font-mono text-xs'>
              {formatQuota(r?.bonus_shortfall)}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  )
}

function OkPanel({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  return (
    <div className='bg-card space-y-4 rounded-xl border border-emerald-200 p-5 shadow-xs dark:border-emerald-900/40'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge status={data.status} />
        {data.balanced ? (
          <Badge variant='secondary'>{t('Balanced')}</Badge>
        ) : null}
        {data.skipped ? (
          <Badge variant='outline'>{t('Skipped (already balanced that day)')}</Badge>
        ) : null}
      </div>
      <div className='grid gap-2 sm:grid-cols-2'>
        <MetaRow label={t('Ledger sum')} value={formatQuota(data.ledger_sum)} />
        <MetaRow
          label={t('Unrolled consume')}
          value={formatQuota(data.unrolled_consume)}
        />
        <MetaRow
          label={t('Wallet paid sum')}
          value={formatQuota(data.wallet_paid_sum)}
        />
        <MetaRow
          label={t('Wallet bonus sum')}
          value={formatQuota(data.wallet_bonus_sum)}
        />
        <MetaRow label={t('Diff')} value={formatQuota(data.diff ?? 0)} />
        <AccountDriftRow data={data} />
      </div>
      <div>
        <h3 className='mb-2 text-sm font-medium'>
          {t('Refund three-way check')}
        </h3>
        <RefundTable data={data} />
      </div>
    </div>
  )
}

function MismatchPanel({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  return (
    <div className='space-y-4 rounded-xl border border-red-200 bg-red-50/50 p-5 shadow-xs dark:border-red-900/50 dark:bg-red-950/20'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge status={data.status} />
        <span className='text-sm font-medium text-red-950 dark:text-red-100'>
          {t('Ledger and wallet totals do not match')}
        </span>
      </div>
      <div className='grid gap-2 sm:grid-cols-2'>
        <MetaRow label={t('Diff')} value={formatQuota(data.diff)} />
        <MetaRow label={t('Ledger sum')} value={formatQuota(data.ledger_sum)} />
        <MetaRow
          label={t('Unrolled consume')}
          value={formatQuota(data.unrolled_consume)}
        />
        <MetaRow
          label={t('Wallet paid sum')}
          value={formatQuota(data.wallet_paid_sum)}
        />
        <MetaRow
          label={t('Wallet bonus sum')}
          value={formatQuota(data.wallet_bonus_sum)}
        />
        <MetaRow
          label={t('Mismatch users')}
          value={formatQuota(data.mismatch_users)}
        />
        <AccountDriftRow data={data} />
      </div>
      <MetaRow
        label={t('Order nos')}
        value={
          data.order_nos && data.order_nos.length > 0 ? (
            <span className='break-all'>{data.order_nos.join(', ')}</span>
          ) : (
            '—'
          )
        }
      />
      <MetaRow
        label={t('Mismatch user ids')}
        value={
          data.mismatch_user_ids && data.mismatch_user_ids.length > 0 ? (
            <span className='break-all'>
              {data.mismatch_user_ids.join(', ')}
            </span>
          ) : (
            '—'
          )
        }
      />
      <div>
        <h3 className='mb-2 text-sm font-medium'>
          {t('Refund three-way check')}
        </h3>
        <RefundTable data={data} />
      </div>
    </div>
  )
}

function ResultBody({ data }: { data: ReconcileResult }) {
  const { t } = useTranslation()
  const backup = formatLastBackup(data.last_backup)

  return (
    <div className='space-y-4'>
      <div className='bg-card space-y-2 rounded-xl border p-4 shadow-xs'>
        <div className='flex flex-wrap items-center gap-2'>
          <StatusBadge status={data.status} />
          <span className='text-muted-foreground text-xs'>
            {data.date} ·{' '}
            {(data.timezone || 'Asia/Shanghai') === 'Asia/Shanghai'
              ? t('Beijing time')
              : data.timezone}
          </span>
        </div>
        <MetaRow
          label={t('Last backup')}
          value={
            backup === 'none' ? (
              <span className='text-muted-foreground'>
                {t('Never succeeded')}
              </span>
            ) : (
              backup
            )
          }
        />
        <MetaRow
          label={t('Backfill status')}
          value={t(backfillStatusLabelKey(data.backfill_status))}
        />
        {data.note && !showFailedNote(data.status) ? (
          <MetaRow label={t('Note')} value={data.note} />
        ) : null}
      </div>

      {showIncompleteOnly(data.status) ? <IncompletePanel data={data} /> : null}
      {showFailedNote(data.status) ? <FailedPanel data={data} /> : null}
      {showOkSummary(data.status) ? <OkPanel data={data} /> : null}
      {showMismatchDetails(data.status) ? <MismatchPanel data={data} /> : null}

      {!showIncompleteOnly(data.status) &&
      !showFailedNote(data.status) &&
      !showOkSummary(data.status) &&
      !showMismatchDetails(data.status) ? (
        <div className='bg-card rounded-xl border p-5 text-sm shadow-xs'>
          <StatusBadge status={data.status} />
          {data.note ? (
            <p className='mt-2 font-mono text-xs break-all'>{data.note}</p>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

export function ReconcileSettings() {
  const { t } = useTranslation()
  const [draftDate, setDraftDate] = useState(() => defaultReconcileDate())
  const [queryDate, setQueryDate] = useState(() => defaultReconcileDate())
  const reconcileQuery = useReconcile(queryDate)

  const isForbidden =
    (reconcileQuery.error as { status?: number } | null)?.status === 403

  function onQuery() {
    if (draftDate === queryDate) {
      void reconcileQuery.refetch()
      return
    }
    setQueryDate(draftDate)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Daily reconcile')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='flex flex-wrap items-center gap-2'>
          <Input
            type='date'
            className='h-8 w-[11.5rem]'
            value={draftDate}
            onChange={(e) => setDraftDate(e.target.value)}
            aria-label={t('Business date')}
          />
          <Button
            type='button'
            size='sm'
            className='h-8'
            onClick={() => onQuery()}
            disabled={reconcileQuery.isFetching || !draftDate}
          >
            <Search className='size-3.5' />
            {reconcileQuery.isFetching ? t('Querying...') : t('Query')}
          </Button>
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Daily ledger reconcile for a Shanghai business day. Defaults to yesterday (Asia/Shanghai). Incomplete means hour buckets are missing — the UI withholds a full result.'
            )}
          </p>

          <div className='grid gap-3'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'GET /api/admin/reconcile may backfill missing hour buckets. Prefer a deliberate Query click when changing dates.'
                )}
              </div>
            </div>
          </div>

          {reconcileQuery.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading settings...')}
            </div>
          ) : null}

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'Admin permission is required to view daily reconcile.'
              )}
            />
          ) : null}

          {reconcileQuery.isError && !reconcileQuery.data && !isForbidden ? (
            <ErrorState
              title={t('Failed to load settings')}
              onRetry={() => void reconcileQuery.refetch()}
            />
          ) : null}

          {reconcileQuery.data ? (
            <ResultBody data={reconcileQuery.data} />
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
