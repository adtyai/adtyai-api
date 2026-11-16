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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { siteWithdrawalStatusLabel } from '@/features/adty-ledger/site-wallet-labels'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { invalidateSidebarBadges } from '@/hooks/use-sidebar-badges'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getAdminSiteWithdrawals,
  markSiteWithdrawalPaid,
  markSiteWithdrawalPaying,
  rejectSiteWithdrawal,
  siteAdminKeys,
  type AdminSiteWithdrawal,
} from '../api'

const PAGE_SIZE = 20
const STATUSES = ['pending', 'paying', 'paid', 'rejected', 'cancelled'] as const

type Action =
  | { kind: 'paying'; row: AdminSiteWithdrawal }
  | { kind: 'paid'; row: AdminSiteWithdrawal }
  | { kind: 'reject'; row: AdminSiteWithdrawal }

function statusVariant(status: string) {
  if (status === 'paid') return 'success' as const
  if (status === 'pending' || status === 'paying') return 'warning' as const
  if (status === 'rejected') return 'danger' as const
  return 'neutral' as const
}

/**
 * Root reviews site earnings withdrawals: pending → paying → paid, or reject
 * (pending or paying), which returns the amount to the earnings wallet. Money
 * moves offline; USDT payouts need the transaction hash.
 */
export function SiteWithdrawalsPanel() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [status, setStatus] = useState('pending')
  const [page, setPage] = useState(1)
  const [action, setAction] = useState<Action | null>(null)
  const [txId, setTxId] = useState('')
  const [proof, setProof] = useState('')
  const [reason, setReason] = useState('')
  const query = useQuery({
    queryKey: siteAdminKeys.list('site-withdrawals', { status, page }),
    queryFn: () => getAdminSiteWithdrawals(status, page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  const rows = Array.isArray(query.data?.items) ? query.data.items : []
  const open = (next: Action) => {
    setTxId('')
    setProof('')
    setReason('')
    setAction(next)
  }
  const mutation = useMutation({
    mutationFn: async (next: Action) => {
      if (next.kind === 'paying') return markSiteWithdrawalPaying(next.row.id)
      if (next.kind === 'paid') {
        return markSiteWithdrawalPaid(next.row.id, {
          tx_id: txId.trim(),
          proof: proof.trim(),
        })
      }
      return rejectSiteWithdrawal(next.row.id, reason.trim())
    },
    onSuccess: (_data, next) => {
      const done = {
        paying: t('Marked as paying'),
        paid: t('Payout recorded'),
        reject: t('Withdrawal rejected'),
      }
      toast.success(done[next.kind])
      setAction(null)
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
      void invalidateSidebarBadges(client)
    },
    onError: (error) => handleServerError(error),
  })
  const paidReady =
    action?.kind === 'paid' &&
    (action.row.method === 'usdt'
      ? txId.trim() !== ''
      : txId.trim() !== '' || proof.trim() !== '')

  return (
    <section className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Site withdrawals')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Pay site owners offline, then record it here. Rejecting returns the amount to the site earnings wallet.'
            )}
          </p>
        </div>
        <NativeSelect
          aria-label={t('Status')}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value)
            setPage(1)
          }}
        >
          <NativeSelectOption value=''>{t('All')}</NativeSelectOption>
          {STATUSES.map((value) => (
            <NativeSelectOption key={value} value={value}>
              {siteWithdrawalStatusLabel(t, value)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isPending ? (
        <LoadingState />
      ) : (
        query.data && (
          <>
            <StaticDataTable
              tableClassName='min-w-[960px]'
              tableProps={{ 'aria-label': t('Site withdrawals') }}
              data={rows}
              getRowKey={(row) => String(row.id)}
              emptyContent={
                <EmptyState className='min-h-24' title={t('No withdrawals')} />
              }
              columns={[
                {
                  id: 'time',
                  header: t('Requested at'),
                  cell: (row) => formatTimestamp(row.created_at),
                },
                {
                  id: 'site',
                  header: t('Site'),
                  cell: (row) => (
                    <div className='flex flex-col'>
                      <span>{row.subdomain || `#${row.site_id}`}</span>
                      <span className='text-muted-foreground text-xs'>
                        {t('Owner')} #{row.owner_user_id}
                      </span>
                    </div>
                  ),
                },
                {
                  id: 'amount',
                  header: t('Amount'),
                  cell: (row) => (
                    <div className='flex flex-col tabular-nums'>
                      <span>{row.amount_usd} USD</span>
                      <span className='text-muted-foreground text-xs'>
                        {t('Withdrawal fee')} {row.fee_usd} · {t('Net')}{' '}
                        {row.net_usd}
                      </span>
                    </div>
                  ),
                },
                {
                  id: 'payout',
                  header: t('Payout'),
                  cell: (row) => (
                    <div className='flex flex-col tabular-nums'>
                      <span>
                        {row.payout_amount} {row.payout_currency}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {t('Exchange rate')} {row.rate}
                      </span>
                    </div>
                  ),
                },
                {
                  id: 'account',
                  header: t('Payee'),
                  cell: (row) => (
                    <div className='flex flex-col gap-0.5'>
                      <span className='font-medium'>
                        {row.method === 'usdt'
                          ? `USDT · ${row.network}`
                          : t('Alipay')}
                      </span>
                      <div className='flex items-center gap-1'>
                        <span className='font-mono text-xs break-all'>
                          {row.account_no}
                        </span>
                        <CopyButton
                          value={row.account_no}
                          size='icon'
                          aria-label={t('Copy account')}
                        />
                      </div>
                      {row.account_name && (
                        <span className='text-muted-foreground text-xs'>
                          {row.account_name}
                        </span>
                      )}
                    </div>
                  ),
                },
                {
                  id: 'status',
                  header: t('Status'),
                  cell: (row) => (
                    <div className='flex flex-col gap-0.5'>
                      <StatusBadge
                        label={siteWithdrawalStatusLabel(t, row.status)}
                        variant={statusVariant(row.status)}
                        copyable={false}
                      />
                      {(row.tx_id || row.proof_note) && (
                        <span className='text-muted-foreground font-mono text-xs break-all'>
                          {row.tx_id || row.proof_note}
                        </span>
                      )}
                    </div>
                  ),
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  className: 'text-right',
                  cellClassName: 'text-right',
                  cell: (row) => (
                    <div className='flex justify-end gap-1'>
                      {row.status === 'pending' && (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => open({ kind: 'paying', row })}
                        >
                          {t('Mark paying')}
                        </Button>
                      )}
                      {row.status === 'paying' && (
                        <Button
                          size='sm'
                          onClick={() => open({ kind: 'paid', row })}
                        >
                          {t('Record payout')}
                        </Button>
                      )}
                      {(row.status === 'pending' ||
                        row.status === 'paying') && (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => open({ kind: 'reject', row })}
                        >
                          {t('Reject')}
                        </Button>
                      )}
                    </div>
                  ),
                },
              ]}
            />
            <PageSwitcher
              page={page}
              total={Number(query.data.total) || 0}
              pageSize={PAGE_SIZE}
              onPageChange={setPage}
            />
          </>
        )
      )}
      <ConfirmDialog
        open={action?.kind === 'paying'}
        onOpenChange={(value) => {
          if (!value && !mutation.isPending) setAction(null)
        }}
        title={t('Mark as paying')}
        desc={t(
          'The owner can no longer cancel this request. Record the payout after the money is sent.'
        )}
        confirmText={t('Mark paying')}
        isLoading={mutation.isPending}
        handleConfirm={() => action && mutation.mutate(action)}
      />
      <Dialog
        open={action?.kind === 'paid' || action?.kind === 'reject'}
        onOpenChange={(value) => {
          if (!value && !mutation.isPending) setAction(null)
        }}
        title={
          action?.kind === 'paid' ? t('Record payout') : t('Reject withdrawal')
        }
        description={
          action?.kind === 'paid'
            ? t(
                'Enter the transaction hash, or for Alipay the transfer reference. USDT payouts require the transaction hash.'
              )
            : t('The amount returns to the site earnings wallet.')
        }
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              variant='outline'
              disabled={mutation.isPending}
              onClick={() => setAction(null)}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant={action?.kind === 'reject' ? 'destructive' : 'brand'}
              disabled={
                mutation.isPending ||
                (action?.kind === 'paid' ? !paidReady : reason.trim() === '')
              }
              onClick={() => action && mutation.mutate(action)}
            >
              {action?.kind === 'paid' ? t('Record payout') : t('Reject')}
            </Button>
          </>
        }
      >
        {action && (
          <p className='text-sm tabular-nums'>
            {action.row.subdomain || `#${action.row.site_id}`} ·{' '}
            {action.row.payout_amount} {action.row.payout_currency}
          </p>
        )}
        {action?.kind === 'paid' ? (
          <>
            <div className='space-y-2'>
              <Label htmlFor='site-withdraw-tx'>{t('Transaction hash')}</Label>
              <Input
                id='site-withdraw-tx'
                value={txId}
                onChange={(event) => setTxId(event.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='site-withdraw-proof'>
                {t('Transfer reference or note')}
              </Label>
              <Input
                id='site-withdraw-proof'
                value={proof}
                onChange={(event) => setProof(event.target.value)}
              />
            </div>
          </>
        ) : (
          <div className='space-y-2'>
            <Label htmlFor='site-withdraw-reason'>
              {t('Reason (required)')}
            </Label>
            <Textarea
              id='site-withdraw-reason'
              value={reason}
              rows={3}
              onChange={(event) => setReason(event.target.value)}
            />
          </div>
        )}
      </Dialog>
    </section>
  )
}
