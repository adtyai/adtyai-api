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
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

import {
  ADMIN_REFERRAL_PAGE_SIZE,
  adminReferralKeys,
  getCommissionJobs,
  getReferralAuditLogs,
  getReferralLedgers,
  retryCommissionJob,
} from '../api'

const JOB_STATUSES = [
  'failed',
  'pending',
  'processing',
  'succeeded',
  'skipped',
] as const

/** Literal t() calls so the key scanner sees every label. */
function jobStatusLabel(t: TFunction, status: string) {
  switch (status) {
    case 'pending':
      return t('Waiting')
    case 'processing':
      return t('Processing')
    case 'succeeded':
      return t('Succeeded')
    case 'skipped':
      return t('Skipped')
    case 'failed':
      return t('Failed')
    default:
      return status
  }
}

function ledgerTypeLabel(t: TFunction, type: string) {
  switch (type) {
    case 'commission_accrue':
      return t('Commission accrued')
    case 'commission_settle':
      return t('Commission settled')
    case 'commission_void':
      return t('Commission voided by refund')
    case 'commission_clawback':
      return t('Commission clawback')
    case 'commission_clawback_partial':
      return t('Partial commission clawback')
    case 'withdraw_freeze':
      return t('Withdrawal requested')
    case 'withdraw_to_balance':
      return t('Transferred to balance')
    case 'withdraw_release':
      return t('Withdrawal returned')
    case 'withdraw_paid':
      return t('Withdrawal paid')
    case 'admin_adjust':
      return t('Admin adjustment')
    default:
      return type
  }
}

function auditActionLabel(t: TFunction, action: string) {
  switch (action) {
    case 'partner_approve':
      return t('Approved partner application')
    case 'partner_reject':
      return t('Rejected partner application')
    case 'withdrawal_approve':
      return t('Approved withdrawal')
    case 'withdrawal_reject':
      return t('Rejected withdrawal')
    case 'withdrawal_pay':
      return t('Marked withdrawal paid')
    case 'withdraw_cny_rate':
      return t('Changed withdrawal CNY rate')
    case 'rate':
      return t('Changed affiliate rate')
    case 'disable':
      return t('Disabled affiliate')
    case 'restore':
      return t('Restored affiliate')
    case 'freeze_settlement':
      return t('Froze settlement')
    case 'restore_settlement':
      return t('Restored settlement')
    case 'freeze_withdrawal':
      return t('Froze withdrawals')
    case 'restore_withdrawal':
      return t('Restored withdrawals')
    case 'restore_referral':
      return t('Restored referral commission')
    case 'adjust':
      return t('Adjusted commission')
    default:
      return action
  }
}

function jobVariant(status: string) {
  if (status === 'succeeded') return 'success' as const
  if (status === 'failed') return 'danger' as const
  if (status === 'pending' || status === 'processing') return 'warning' as const
  return 'neutral' as const
}

/** A signed USD delta; zero shows as a dash so the moving column stands out. */
function Delta(props: { value: number }) {
  const value = Number(props.value) || 0
  if (value === 0) return <span className='text-muted-foreground'>—</span>
  return (
    <span className={cn('tabular-nums', value < 0 && 'text-destructive')}>
      {value > 0 ? '+' : ''}
      {value}
    </span>
  )
}

/**
 * Commission jobs: one per paid top-up with a referral snapshot. A failed or
 * stuck job can be run again; succeeded and skipped jobs never pay twice.
 */
export function CommissionJobsPanel(props: { canRetry: boolean }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [status, setStatus] = useState('failed')
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: adminReferralKeys.list('commission-jobs', { status, page }),
    queryFn: () => getCommissionJobs({ p: page, status: status || undefined }),
    placeholderData: keepPreviousData,
  })
  const retry = useMutation({
    mutationFn: retryCommissionJob,
    onSuccess: () => {
      toast.success(t('Commission job processed'))
      void client.invalidateQueries({ queryKey: adminReferralKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const rows = Array.isArray(query.data?.items) ? query.data.items : []
  return (
    <section className='flex flex-col gap-3 pt-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <p className='text-muted-foreground max-w-2xl text-sm'>
          {t(
            'Each paid top-up with a referral snapshot gets one commission job. Run a failed or stuck job again after fixing its cause; a job that already paid or was skipped is never paid twice.'
          )}
        </p>
        <NativeSelect
          aria-label={t('Status')}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value)
            setPage(1)
          }}
        >
          <NativeSelectOption value=''>{t('All')}</NativeSelectOption>
          {JOB_STATUSES.map((value) => (
            <NativeSelectOption key={value} value={value}>
              {jobStatusLabel(t, value)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isPending ? (
        <LoadingState />
      ) : (
        <>
          <StaticDataTable
            tableClassName='min-w-[860px]'
            tableProps={{ 'aria-label': t('Commission jobs') }}
            data={rows}
            getRowKey={(row) => String(row.id)}
            emptyContent={
              <EmptyState
                className='min-h-24'
                title={t('No commission jobs')}
              />
            }
            columns={[
              {
                id: 'time',
                header: t('Created at'),
                cell: (row) => formatTimestamp(row.created_at),
              },
              {
                id: 'order',
                header: t('Order number'),
                cell: (row) => (
                  <code className='font-mono text-xs break-all'>
                    {row.source_trade_no}
                  </code>
                ),
              },
              {
                id: 'affiliate',
                header: t('Affiliate'),
                cell: (row) =>
                  row.affiliate_id ? `#${row.affiliate_id}` : '—',
              },
              {
                id: 'status',
                header: t('Status'),
                cell: (row) => (
                  <StatusBadge
                    label={jobStatusLabel(t, row.status)}
                    variant={jobVariant(row.status)}
                    copyable={false}
                  />
                ),
              },
              {
                id: 'attempts',
                header: t('Attempts'),
                cellClassName: 'tabular-nums',
                cell: (row) => row.attempt_count,
              },
              {
                id: 'error',
                header: t('Last result'),
                cell: (row) => (
                  <span className='text-muted-foreground text-xs break-words'>
                    {row.last_error || '—'}
                  </span>
                ),
              },
              {
                id: 'actions',
                header: t('Actions'),
                className: 'text-right',
                cellClassName: 'text-right',
                cell: (row) =>
                  props.canRetry &&
                  row.status !== 'succeeded' &&
                  row.status !== 'skipped' ? (
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={retry.isPending}
                      onClick={() => retry.mutate(row.source_trade_no)}
                    >
                      {t('Run again')}
                    </Button>
                  ) : null,
              },
            ]}
          />
          <PageSwitcher
            page={page}
            total={Number(query.data?.total) || 0}
            pageSize={ADMIN_REFERRAL_PAGE_SIZE}
            onPageChange={setPage}
          />
        </>
      )}
    </section>
  )
}

/** Every movement of affiliate commission accounts, newest first. */
export function LedgerPanel() {
  const { t } = useTranslation()
  const [affiliate, setAffiliate] = useState('')
  const [page, setPage] = useState(1)
  const affiliateId = /^\d+$/.test(affiliate.trim())
    ? Number(affiliate.trim())
    : undefined
  const query = useQuery({
    queryKey: adminReferralKeys.list('ledgers', { affiliateId, page }),
    queryFn: () => getReferralLedgers({ p: page, affiliate_id: affiliateId }),
    placeholderData: keepPreviousData,
  })
  const rows = Array.isArray(query.data?.items) ? query.data.items : []
  return (
    <section className='flex flex-col gap-3 pt-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <p className='text-muted-foreground max-w-2xl text-sm'>
          {t(
            'Every change to affiliate commission accounts in USD: settling, withdrawable, withdrawing and withdrawn.'
          )}
        </p>
        <Input
          aria-label={t('Affiliate ID')}
          placeholder={t('Affiliate ID')}
          inputMode='numeric'
          className='w-40'
          value={affiliate}
          onChange={(event) => {
            setAffiliate(event.target.value)
            setPage(1)
          }}
        />
      </div>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isPending ? (
        <LoadingState />
      ) : (
        <>
          <StaticDataTable
            tableClassName='min-w-[980px]'
            tableProps={{ 'aria-label': t('Commission ledger') }}
            data={rows}
            getRowKey={(row) => String(row.id)}
            emptyContent={
              <EmptyState className='min-h-24' title={t('No records')} />
            }
            columns={[
              {
                id: 'time',
                header: t('Time'),
                cell: (row) => formatTimestamp(row.created_at),
              },
              {
                id: 'affiliate',
                header: t('Affiliate'),
                cell: (row) => (
                  <div className='flex flex-col text-xs'>
                    <span>#{row.affiliate_id}</span>
                    <span className='text-muted-foreground'>
                      {t('User')} #{row.user_id}
                    </span>
                  </div>
                ),
              },
              {
                id: 'type',
                header: t('Type'),
                cell: (row) => ledgerTypeLabel(t, row.type),
              },
              {
                id: 'pending',
                header: t('Settling'),
                cell: (row) => <Delta value={row.delta_pending} />,
              },
              {
                id: 'available',
                header: t('Withdrawable'),
                cell: (row) => <Delta value={row.delta_available} />,
              },
              {
                id: 'frozen',
                header: t('Withdrawing'),
                cell: (row) => <Delta value={row.delta_frozen} />,
              },
              {
                id: 'withdrawn',
                header: t('Withdrawn'),
                cell: (row) => <Delta value={row.delta_withdrawn} />,
              },
              {
                id: 'reference',
                header: t('Reference'),
                cell: (row) =>
                  row.ref_id ? (
                    <code className='font-mono text-xs break-all'>
                      {row.ref_id}
                    </code>
                  ) : (
                    '—'
                  ),
              },
              {
                id: 'remark',
                header: t('Note'),
                cell: (row) => (
                  <span className='text-muted-foreground text-xs break-words'>
                    {row.remark || '—'}
                  </span>
                ),
              },
              {
                id: 'operator',
                header: t('Operator'),
                cell: (row) =>
                  !row.operator || row.operator === 'system'
                    ? t('System')
                    : `#${row.operator}`,
              },
            ]}
          />
          <PageSwitcher
            page={page}
            total={Number(query.data?.total) || 0}
            pageSize={ADMIN_REFERRAL_PAGE_SIZE}
            onPageChange={setPage}
          />
        </>
      )}
    </section>
  )
}

/** Shortens a stored JSON snapshot for the table; the full value stays in the title. */
function snippet(value: string) {
  const text = (value || '').trim()
  if (!text || text === 'null' || text === '{}') return ''
  return text.length > 120 ? `${text.slice(0, 120)}…` : text
}

/** Referral admin actions: partner and withdrawal reviews, affiliate changes, rate changes. */
export function AuditLogPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: adminReferralKeys.list('audit-logs', { page }),
    queryFn: () => getReferralAuditLogs({ p: page }),
    placeholderData: keepPreviousData,
  })
  const rows = Array.isArray(query.data?.items) ? query.data.items : []
  return (
    <section className='flex flex-col gap-3 pt-4'>
      <p className='text-muted-foreground max-w-2xl text-sm'>
        {t(
          'Who reviewed partner applications and withdrawals, changed affiliates, adjusted commission or changed the withdrawal rate.'
        )}
      </p>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isPending ? (
        <LoadingState />
      ) : (
        <>
          <StaticDataTable
            tableClassName='min-w-[900px]'
            tableProps={{ 'aria-label': t('Admin activity') }}
            data={rows}
            getRowKey={(row) => String(row.id)}
            emptyContent={
              <EmptyState className='min-h-24' title={t('No records')} />
            }
            columns={[
              {
                id: 'time',
                header: t('Time'),
                cell: (row) => formatTimestamp(row.created_at),
              },
              {
                id: 'action',
                header: t('Action'),
                cell: (row) => auditActionLabel(t, row.action),
              },
              {
                id: 'admin',
                header: t('Operator'),
                cell: (row) => `#${row.admin_user_id}`,
              },
              {
                id: 'target',
                header: t('Target'),
                cell: (row) =>
                  row.target_user_id
                    ? `${t('User')} #${row.target_user_id}`
                    : '—',
              },
              {
                id: 'reason',
                header: t('Reason'),
                cell: (row) => (
                  <span className='text-xs break-words'>
                    {row.reason || '—'}
                  </span>
                ),
              },
              {
                id: 'change',
                header: t('Change'),
                cell: (row) => {
                  const before = snippet(row.old_value)
                  const after = snippet(row.new_value)
                  if (!before && !after) return '—'
                  return (
                    <code
                      className='text-muted-foreground font-mono text-xs break-all'
                      title={`${row.old_value} → ${row.new_value}`}
                    >
                      {before ? `${before} → ` : ''}
                      {after}
                    </code>
                  )
                },
              },
              {
                id: 'ip',
                header: t('IP'),
                cell: (row) => (
                  <span className='font-mono text-xs'>{row.ip || '—'}</span>
                ),
              },
            ]}
          />
          <PageSwitcher
            page={page}
            total={Number(query.data?.total) || 0}
            pageSize={ADMIN_REFERRAL_PAGE_SIZE}
            onPageChange={setPage}
          />
        </>
      )}
    </section>
  )
}
