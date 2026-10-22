import { Info, RefreshCw, ShieldAlert } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { formatAffUsd } from '@/features/affiliate/lib/quota'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import {
  adminActionsForWithdrawStatus,
  withdrawMethodLabel,
  withdrawStatusLabel,
  type AffAdminWithdrawAction,
} from '@/features/affiliate/lib/status'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { usdtChainById } from '@/lib/usdt-chains'
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
import { AffUserDetailButton } from './user-detail-dialog'
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
  const [payMethod, setPayMethod] = useState('all')
  const [keyword, setKeyword] = useState('')
  const [offset, setOffset] = useState(0)
  const [qrUrl, setQrUrl] = useState<string | null>(null)
  const [inviteStatus, setInviteStatus] = useState('pending')
  const [inviteOffset, setInviteOffset] = useState(0)
  const pageSize = 50
  const list = useAdminAffWithdrawals({
    status: status === 'all' ? '' : status,
    payMethod: payMethod === 'all' ? '' : payMethod,
    keyword: keyword.trim(),
    offset,
  })
  const invites = useAdminAffSelfInviteReviews(
    inviteStatus === 'all' ? '' : inviteStatus,
    inviteOffset
  )
  const [target, setTarget] = useState<AffConfirmKind | null>(null)
  const isForbidden = (list.error as { status?: number } | null)?.status === 403
  const rows = list.data?.rows ?? []

  const counts = list.data?.counts ?? {}

  useEffect(() => {
    return () => {
      if (qrUrl) URL.revokeObjectURL(qrUrl)
    }
  }, [qrUrl])

  async function openQr(id: number) {
    try {
      const res = await api.get(`/api/admin/aff/payout-qr/${id}`, {
        responseType: 'blob',
        validateStatus: (s) => s === 200 || s === 403 || s === 404,
      })
      const contentType = String(res.headers['content-type'] || '')
      if (res.status !== 200 || !contentType.startsWith('image/')) {
        toast.error(t('Failed to load payout QR'))
        return
      }
      if (qrUrl) URL.revokeObjectURL(qrUrl)
      setQrUrl(URL.createObjectURL(res.data))
    } catch (err) {
      toast.error(getServerErrorMessage(err, t('Failed to load payout QR')))
    }
  }

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
            if (v) {
              setStatus(v)
              setOffset(0)
            }
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
        <Select
          items={[
            { value: 'all', label: t('All methods') },
            { value: 'usdt', label: 'USDT' },
            { value: 'alipay', label: t('Alipay') },
            { value: 'legacy', label: t('Legacy payout account') },
          ]}
          value={payMethod}
          onValueChange={(v) => {
            if (v) {
              setPayMethod(v)
              setOffset(0)
            }
          }}
        >
          <SelectTrigger className='h-8 w-40 text-sm'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('All methods')}</SelectItem>
            <SelectItem value='usdt'>USDT</SelectItem>
            <SelectItem value='alipay'>{t('Alipay')}</SelectItem>
            <SelectItem value='legacy'>
              {t('Legacy payout account')}
            </SelectItem>
          </SelectContent>
        </Select>
        <Input
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value)
            setOffset(0)
          }}
          placeholder={t('Keyword')}
          className='h-8 w-40 text-sm'
        />
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

          <div className='grid gap-3 sm:grid-cols-3'>
            {(
              [
                ['pending', t('Pending'), counts.pending ?? 0],
                ['approved', t('Approved'), counts.approved ?? 0],
                ['failed', t('Failed payouts'), counts.failed ?? 0],
              ] as [string, string, number][]
            ).map(([id, label, n]) => (
              <button
                key={id}
                type='button'
                className='bg-card rounded-xl border p-3 text-left shadow-xs'
                onClick={() => {
                  if (id !== 'failed') {
                    setStatus(id)
                    setOffset(0)
                  }
                }}
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
                    <TableHead>{t('Payout method')}</TableHead>
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
                        colSpan={9}
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
                          <TableCell>
                            <div>{row.username || row.user_id}</div>
                            <div className='text-muted-foreground text-[11px]'>
                              {row.user_id}
                            </div>
                            {row.user_id > 0 ? (
                              <AffUserDetailButton userId={row.user_id} />
                            ) : null}
                          </TableCell>
                          <TableCell className='font-semibold'>
                            {formatAffUsd(row.amount_quota)}
                          </TableCell>
                          <TableCell>{formatAffUsd(row.fee_quota)}</TableCell>
                          <TableCell>
                            <div>{t(withdrawMethodLabel(row))}</div>
                            {row.usdt_chain ? (
                              <div className='text-muted-foreground text-[11px]'>
                                {usdtChainById(row.usdt_chain)?.label ||
                                  row.usdt_chain}
                              </div>
                            ) : null}
                            {row.payout_qr_id ? (
                              <Button
                                type='button'
                                variant='link'
                                className='h-auto px-0 text-xs'
                                onClick={() => void openQr(row.payout_qr_id || 0)}
                              >
                                {t('Payout QR')}
                              </Button>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <span className='font-mono text-[12px] break-all'>
                              {row.pay_account || '-'}
                            </span>
                            {row.payout_ref ? (
                              <div className='text-muted-foreground text-[11px] break-all'>
                                {row.payout_ref}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <code className='text-xs'>
                              {t(withdrawStatusLabel(row.status))}
                            </code>
                          </TableCell>
                          <TableCell>
                            <span className='text-muted-foreground line-clamp-2 text-xs'>
                              {row.reject_reason || row.payout_ref || '-'}
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
              <div className='flex items-center justify-between border-t px-5 py-3 text-xs'>
                <span className='text-muted-foreground'>
                  {t('Counts ignore the current filter')}
                </span>
                {(list.data?.total ?? 0) > pageSize ? (
                  <div className='flex items-center gap-2'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7'
                      disabled={offset <= 0}
                      onClick={() => setOffset(Math.max(0, offset - pageSize))}
                    >
                      {t('Previous')}
                    </Button>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7'
                      disabled={offset + pageSize >= (list.data?.total ?? 0)}
                      onClick={() => setOffset(offset + pageSize)}
                    >
                      {t('Next')}
                    </Button>
                  </div>
                ) : (
                  <span />
                )}
              </div>
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
                  if (v) {
                    setInviteStatus(v)
                    setInviteOffset(0)
                  }
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
                  <TableHead>{t('Inviter')}</TableHead>
                  <TableHead>{t('Invitee')}</TableHead>
                  <TableHead>{t('Order')}</TableHead>
                  <TableHead>{t('Amount')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Reason')}</TableHead>
                  <TableHead className='w-[1%]' />
                </TableRow>
              </TableHeader>
              <TableBody>
                {(invites.data?.rows ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={8}
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
                      <TableCell>
                        {row.inviter_name || row.inviter_id || '-'}
                      </TableCell>
                      <TableCell>
                        {row.invitee_name || row.invitee_id || row.user_id}
                      </TableCell>
                      <TableCell>
                        <code className='text-xs'>{row.order_no || '-'}</code>
                      </TableCell>
                      <TableCell>
                        {formatBillingCurrencyFromUSD(row.amount_usd || 0, {
                          digitsLarge: 2,
                          digitsSmall: 2,
                          abbreviate: false,
                        })}
                      </TableCell>
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
                        ) : null}
                        {row.status === 'rejected' ? (
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            className='h-7 px-2.5 text-xs'
                            onClick={() =>
                              setTarget({
                                kind: 'self-invite',
                                row,
                                action: 'reopen',
                              })
                            }
                          >
                            {t('Restore self-invite review')}
                          </Button>
                        ) : null}
                        {row.status !== 'pending' &&
                        row.status !== 'rejected' ? (
                          <span className='text-muted-foreground text-xs'>
                            -
                          </span>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
            {(invites.data?.total ?? 0) > pageSize ? (
              <div className='flex justify-end gap-2 border-t px-5 py-3'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='h-7'
                  disabled={inviteOffset <= 0}
                  onClick={() =>
                    setInviteOffset(Math.max(0, inviteOffset - pageSize))
                  }
                >
                  {t('Previous')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='h-7'
                  disabled={
                    inviteOffset + pageSize >= (invites.data?.total ?? 0)
                  }
                  onClick={() => setInviteOffset(inviteOffset + pageSize)}
                >
                  {t('Next')}
                </Button>
              </div>
            ) : null}
          </section>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>

      <Dialog
        open={Boolean(qrUrl)}
        onOpenChange={(open) => {
          if (!open) {
            if (qrUrl) URL.revokeObjectURL(qrUrl)
            setQrUrl(null)
          }
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('Payout QR')}</DialogTitle>
          </DialogHeader>
          {qrUrl ? (
            <img
              src={qrUrl}
              alt={t('Payout QR')}
              className='max-h-[420px] w-full object-contain'
            />
          ) : null}
        </DialogContent>
      </Dialog>
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
