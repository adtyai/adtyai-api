import { Info, RefreshCw, ShieldAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
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
import { formatAffUsd } from '@/features/affiliate/lib/quota'
import {
  adminActionsForWithdrawStatus,
  withdrawStatusLabel,
  type AffAdminWithdrawAction,
} from '@/features/affiliate/lib/status'
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
  riskReasonLabelKey,
  riskReviewStatusLabelKey,
} from '@/features/system-settings/utils/status-labels'

import {
  ConfirmAffActionDialog,
  type AffConfirmKind,
} from './components/confirm-aff-action-dialog'
import {
  useAdminAffSelfInviteReviews,
  useAdminAffWithdrawals,
} from './hooks/use-admin-aff'
import type { AffWithdrawal } from './types'

function actionLabel(t: (k: string) => string, action: AffAdminWithdrawAction) {
  switch (action) {
    case 'approve':
      return t('Approve')
    case 'reject':
      return t('Reject')
    case 'mark-paid':
      return t('Mark paid')
    case 'mark-failed':
      return t('Mark failed')
    case 'mark-unknown':
      return t('Mark unknown')
    case 'retry':
      return t('Retry payout')
    default:
      return action
  }
}

export function AffWithdrawalsSettings() {
  const { t } = useTranslation()
  const [status, setStatus] = useState('pending')
  const [inviteStatus, setInviteStatus] = useState('pending')
  const list = useAdminAffWithdrawals(status === 'all' ? '' : status)
  const invites = useAdminAffSelfInviteReviews(
    inviteStatus === 'all' ? '' : inviteStatus
  )
  const [target, setTarget] = useState<AffConfirmKind | null>(null)
  const isForbidden = (list.error as { status?: number } | null)?.status === 403
  const rows = list.data?.rows ?? []

  const counts = useMemo(() => {
    const all = list.data?.rows ?? []
    const pick = (s: string) => all.filter((r) => r.status === s).length
    return {
      pending: pick('pending'),
      approved: pick('approved'),
      paid: pick('paid'),
      rejected: pick('rejected'),
      pay_failed: pick('pay_failed'),
      pay_unknown: pick('pay_unknown'),
    }
  }, [list.data?.rows])

  function openWithdraw(row: AffWithdrawal, action: AffAdminWithdrawAction) {
    setTarget({ kind: 'withdraw', row, action })
  }

  return (
    <>
      <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Affiliate withdrawals')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Select
          items={[
            { value: 'pending', label: t('Pending') },
            { value: 'approved', label: t('Approved') },
            { value: 'paid', label: t('Paid') },
            { value: 'rejected', label: t('Rejected') },
            { value: 'pay_failed', label: t('Pay failed') },
            { value: 'pay_failed_final', label: t('Pay failed (final)') },
            { value: 'pay_unknown', label: t('Pay unknown') },
            { value: 'all', label: t('All statuses') },
          ]}
          value={status}
          onValueChange={(v) => {
            if (v) setStatus(v)
          }}
        >
          <SelectTrigger className='h-8 w-44 text-sm'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='pending'>{t('Pending')}</SelectItem>
            <SelectItem value='approved'>{t('Approved')}</SelectItem>
            <SelectItem value='paid'>{t('Paid')}</SelectItem>
            <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
            <SelectItem value='pay_failed'>{t('Pay failed')}</SelectItem>
            <SelectItem value='pay_failed_final'>
              {t('Pay failed (final)')}
            </SelectItem>
            <SelectItem value='pay_unknown'>{t('Pay unknown')}</SelectItem>
            <SelectItem value='all'>{t('All statuses')}</SelectItem>
          </SelectContent>
        </Select>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => {
            void list.refetch()
            void invites.refetch()
          }}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Withdrawal review state machine. pay_failed can retry; pay_unknown and pay_failed_final cannot. Mutations take effect on submit.'
            )}
          </p>

          <div className='grid gap-3 sm:grid-cols-3 lg:grid-cols-6'>
            {(
              [
                ['pending', t('Pending'), counts.pending],
                ['approved', t('Approved'), counts.approved],
                ['paid', t('Paid'), counts.paid],
                ['rejected', t('Rejected'), counts.rejected],
                ['pay_failed', t('Pay failed'), counts.pay_failed],
                ['pay_unknown', t('Pay unknown'), counts.pay_unknown],
              ] as [string, string, number][]
            ).map(([id, label, n]) => (
              <button
                key={id}
                type='button'
                className='bg-card rounded-xl border p-3 text-left shadow-xs'
                onClick={() => setStatus(id)}
              >
                <div className='text-muted-foreground text-[12px]'>{label}</div>
                <div className='mt-1 text-xl font-semibold'>{n}</div>
              </button>
            ))}
          </div>

          <div className='grid gap-3 md:grid-cols-2'>
            <div className='flex gap-2.5 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-[13px] leading-5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/40 dark:text-amber-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'pay_failed: confirmed not on-chain, retry allowed. pay_unknown: status unclear — retry forbidden; mark-failed to finalize after chain check.'
                )}
              </div>
            </div>
            <div className='bg-muted/40 text-muted-foreground flex gap-2.5 rounded-lg border px-3.5 py-2.5 text-[13px] leading-5'>
              <ShieldAlert className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Approve, reject, mark paid/failed/unknown and retry are sensitive actions. Submit takes effect immediately.'
                )}
              </div>
            </div>
          </div>

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'You do not have permission to view affiliate withdrawals.'
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
              title={t('Failed to load withdrawals')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
              <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                <div className='text-[15px] font-semibold'>
                  {t('Withdrawals')}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {list.data.total} {t('records')}
                </div>
              </div>
              <Table>
                <TableHeader>
                  <TableRow className='bg-muted/40 hover:bg-muted/40'>
                    <TableHead>ID</TableHead>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Amount')}</TableHead>
                    <TableHead>{t('Fee')}</TableHead>
                    <TableHead>{t('Payout account')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Note')}</TableHead>
                    <TableHead className='w-[1%]' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={8}
                        className='text-muted-foreground h-24 text-center text-sm'
                      >
                        {t('No withdrawals')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const actions = adminActionsForWithdrawStatus(row.status)
                      return (
                        <TableRow key={row.id}>
                          <TableCell>
                            <code className='text-xs'>{row.id}</code>
                          </TableCell>
                          <TableCell>{row.user_id}</TableCell>
                          <TableCell className='font-semibold'>
                            {formatAffUsd(row.amount_quota)}
                          </TableCell>
                          <TableCell>{formatAffUsd(row.fee_quota)}</TableCell>
                          <TableCell>
                            <span className='font-mono text-[12px] break-all'>
                              {row.pay_account || '-'}
                            </span>
                          </TableCell>
                          <TableCell>
                            <code className='text-xs'>
                              {t(withdrawStatusLabel(row.status))}
                            </code>
                          </TableCell>
                          <TableCell>
                            <span className='text-muted-foreground line-clamp-2 text-xs'>
                              {row.reject_reason || '-'}
                            </span>
                          </TableCell>
                          <TableCell>
                            {actions.length === 0 &&
                              (row.status === 'pay_unknown' ||
                                row.status === 'pay_failed_final') && (
                                <span className='text-muted-foreground inline-flex h-7 items-center rounded-lg border px-2.5 text-xs'>
                                  {t('Retry forbidden')}
                                </span>
                              )}
                            {actions.length === 0 &&
                              row.status !== 'pay_unknown' &&
                              row.status !== 'pay_failed_final' && (
                                <span className='text-muted-foreground text-xs'>
                                  -
                                </span>
                              )}
                            {actions.length > 0 && (
                              <div className='flex flex-wrap items-center gap-1.5'>
                                {actions.map((action) => (
                                  <Button
                                    key={action}
                                    type='button'
                                    variant={
                                      action === 'reject' ||
                                      action === 'mark-failed'
                                        ? 'outline'
                                        : 'default'
                                    }
                                    size='sm'
                                    className='h-7 px-2.5 text-xs'
                                    onClick={() => openWithdraw(row, action)}
                                  >
                                    {actionLabel(t, action)}
                                  </Button>
                                ))}
                              </div>
                            )}
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </section>
          ) : null}

          <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
            <div className='flex flex-wrap items-center justify-between gap-2 border-b px-5 pt-4 pb-3'>
              <div>
                <div className='text-[15px] font-semibold'>
                  {t('Self-invite reviews')}
                </div>
                <div className='text-muted-foreground mt-0.5 text-xs'>
                  {t(
                    'kind=aff_self_invite — approve/reject via the risk-reviews API.'
                  )}
                </div>
              </div>
              <Select
                items={[
                  { value: 'pending', label: t('Pending') },
                  { value: 'approved', label: t('Approved') },
                  { value: 'rejected', label: t('Rejected') },
                  { value: 'all', label: t('All statuses') },
                ]}
                value={inviteStatus}
                onValueChange={(v) => {
                  if (v) setInviteStatus(v)
                }}
              >
                <SelectTrigger className='h-8 w-36 text-sm'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='pending'>{t('Pending')}</SelectItem>
                  <SelectItem value='approved'>{t('Approved')}</SelectItem>
                  <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
                  <SelectItem value='all'>{t('All statuses')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Table>
              <TableHeader>
                <TableRow className='bg-muted/40 hover:bg-muted/40'>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('User')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Reason')}</TableHead>
                  <TableHead className='w-[1%]' />
                </TableRow>
              </TableHeader>
              <TableBody>
                {(invites.data?.rows ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className='text-muted-foreground h-20 text-center text-sm'
                    >
                      {t('No self-invite reviews')}
                    </TableCell>
                  </TableRow>
                ) : (
                  (invites.data?.rows ?? []).map((row) => (
                    <TableRow key={row.id}>
                      <TableCell>
                        <code className='text-xs'>{row.id}</code>
                      </TableCell>
                      <TableCell>{row.user_id}</TableCell>
                      <TableCell>
                        <code className='text-xs'>
                          {t(riskReviewStatusLabelKey(row.status))}
                        </code>
                      </TableCell>
                      <TableCell>
                        <span className='text-muted-foreground line-clamp-2 text-xs'>
                          {row.reason
                            ? (() => {
                                const k = riskReasonLabelKey(row.reason)
                                return k ? t(k) : row.reason
                              })()
                            : '-'}
                        </span>
                      </TableCell>
                      <TableCell>
                        {row.status === 'pending' ? (
                          <div className='flex items-center gap-1.5'>
                            <Button
                              type='button'
                              size='sm'
                              className='h-7 px-2.5 text-xs'
                              onClick={() =>
                                setTarget({
                                  kind: 'self-invite',
                                  row,
                                  action: 'approve',
                                })
                              }
                            >
                              {t('Approve')}
                            </Button>
                            <Button
                              type='button'
                              variant='outline'
                              size='sm'
                              className='h-7 px-2.5 text-xs'
                              onClick={() =>
                                setTarget({
                                  kind: 'self-invite',
                                  row,
                                  action: 'reject',
                                })
                              }
                            >
                              {t('Reject')}
                            </Button>
                          </div>
                        ) : (
                          <span className='text-muted-foreground text-xs'>
                            -
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </section>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>

      <ConfirmAffActionDialog
        target={target}
        open={Boolean(target)}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
      />
    </>
  )
}

export default AffWithdrawalsSettings
