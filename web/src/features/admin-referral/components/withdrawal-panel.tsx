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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { uploadReferralAsset } from '@/features/referral/api'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import {
  WITHDRAWAL_STATUS_LABELS,
  WITHDRAWAL_STATUS_VARIANTS,
  WITHDRAWAL_TARGET_LABELS,
} from '@/features/referral/lib/labels'
import type { ReferralWithdrawal } from '@/features/referral/types'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  ADMIN_REFERRAL_PAGE_SIZE,
  adminReferralKeys,
  getAdminWithdrawals,
  payWithdrawal,
  reviewWithdrawal,
} from '../api'
import { payoutText } from '../lib/payout'

function PayeeCell(props: { row: ReferralWithdrawal }) {
  const { t } = useTranslation()
  const row = props.row
  if (row.account_type === 'usdt') {
    return (
      <div className='flex flex-col gap-1'>
        <span className='font-medium'>{row.account_network}</span>
        <div className='flex items-center gap-1'>
          <span className='font-mono text-xs break-all'>{row.account_no}</span>
          <CopyButton
            value={row.account_no}
            size='icon'
            aria-label={t('Copy address')}
          />
        </div>
      </div>
    )
  }
  if (row.account_type === 'balance') {
    return <span>{t('Account balance')}</span>
  }
  return (
    <div className='flex items-center gap-2'>
      {row.qr_image_url && (
        <a href={row.qr_image_url} target='_blank' rel='noreferrer'>
          <img
            src={row.qr_image_url}
            alt={t('Payment QR code')}
            className='h-16 w-16 rounded border object-contain'
          />
        </a>
      )}
      <div className='flex flex-col'>
        <span>{t(WITHDRAWAL_TARGET_LABELS[row.account_type])}</span>
        <span className='font-medium'>{row.account_name}</span>
      </div>
    </div>
  )
}

export function WithdrawalPanel(props: {
  canReview: boolean
  canPayout: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [status, setStatus] = useState('pending')
  const [page, setPage] = useState(1)
  const [rejecting, setRejecting] = useState<ReferralWithdrawal | null>(null)
  const [paying, setPaying] = useState<ReferralWithdrawal | null>(null)
  const [reason, setReason] = useState('')
  const [txnNo, setTxnNo] = useState('')
  const [proofUrl, setProofUrl] = useState('')
  const params = { p: page, status }
  const query = useQuery({
    queryKey: adminReferralKeys.list('withdrawals', params),
    queryFn: () => getAdminWithdrawals(params),
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: adminReferralKeys.all })
  const review = useMutation({
    mutationFn: (input: { row: ReferralWithdrawal; approve: boolean }) =>
      reviewWithdrawal(input.row.id, { approve: input.approve, reason }),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      setRejecting(null)
      setReason('')
      void refresh()
    },
    onError: (err) => handleServerError(err),
  })
  const pay = useMutation({
    mutationFn: (row: ReferralWithdrawal) =>
      payWithdrawal(row.id, {
        txn_no: txnNo.trim(),
        proof_url: proofUrl,
        note: '',
      }),
    onSuccess: () => {
      toast.success(t('Marked as paid'))
      setPaying(null)
      setTxnNo('')
      setProofUrl('')
      void refresh()
    },
    onError: (err) => handleServerError(err),
  })
  const upload = useMutation({
    mutationFn: (file: File) =>
      uploadReferralAsset(file, '/api/user/admin/referral/upload'),
    onSuccess: setProofUrl,
    onError: (err) => handleServerError(err),
  })

  return (
    <div className='flex flex-col gap-3'>
      <NativeSelect
        className='w-48'
        aria-label={t('Status')}
        value={status}
        onChange={(event) => {
          setStatus(event.target.value)
          setPage(1)
        }}
      >
        <option value=''>{t('All statuses')}</option>
        {Object.entries(WITHDRAWAL_STATUS_LABELS).map(([value, label]) => (
          <option key={value} value={value}>
            {t(label)}
          </option>
        ))}
      </NativeSelect>
      <StaticDataTable
        tableClassName='min-w-[1000px]'
        tableProps={{ 'aria-label': t('Withdrawals') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState className='min-h-32' title={t('No withdrawals yet')} />
        }
        columns={[
          {
            id: 'time',
            header: t('Time'),
            cell: (row) => formatTimestamp(row.submitted_at),
          },
          { id: 'user', header: t('User ID'), cell: (row) => row.user_id },
          {
            id: 'payee',
            header: t('Pay to'),
            cell: (row) => <PayeeCell row={row} />,
          },
          {
            id: 'payout',
            header: t('Amount to pay'),
            cell: (row) => (
              <div className='flex flex-col'>
                <span className='font-semibold' data-testid='payout'>
                  {payoutText(row, formatUsd(row.amount, locale))}
                </span>
                <span className='text-muted-foreground text-xs'>
                  {formatUsd(row.amount, locale)}
                </span>
              </div>
            ),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <div className='flex flex-col gap-1'>
                <StatusBadge
                  label={t(WITHDRAWAL_STATUS_LABELS[row.status])}
                  variant={WITHDRAWAL_STATUS_VARIANTS[row.status]}
                />
                {row.payment_txn_no && (
                  <span className='font-mono text-xs break-all'>
                    {row.payment_txn_no}
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
                {props.canReview && row.status === 'pending' && (
                  <>
                    <Button
                      size='sm'
                      disabled={review.isPending}
                      onClick={() => review.mutate({ row, approve: true })}
                    >
                      {t('Approve')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setRejecting(row)}
                    >
                      {t('Reject')}
                    </Button>
                  </>
                )}
                {props.canPayout && row.status === 'approved' && (
                  <Button size='sm' onClick={() => setPaying(row)}>
                    {t('Mark as paid')}
                  </Button>
                )}
              </div>
            ),
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={ADMIN_REFERRAL_PAGE_SIZE}
        onPageChange={setPage}
      />

      <Dialog
        open={rejecting !== null}
        onOpenChange={(open) => !open && setRejecting(null)}
        title={t('Reject the withdrawal')}
        footer={
          <Button
            variant='destructive'
            disabled={reason.trim() === '' || review.isPending}
            onClick={() =>
              rejecting && review.mutate({ row: rejecting, approve: false })
            }
          >
            {t('Reject')}
          </Button>
        }
      >
        <div className='space-y-2'>
          <Label htmlFor='reject-reason'>{t('Reason')}</Label>
          <Textarea
            id='reject-reason'
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        </div>
      </Dialog>

      <Dialog
        open={paying !== null}
        onOpenChange={(open) => !open && setPaying(null)}
        title={t('Mark as paid')}
        description={
          paying ? payoutText(paying, formatUsd(paying.amount, locale)) : ''
        }
        footer={
          <Button
            disabled={
              pay.isPending ||
              upload.isPending ||
              (paying?.account_type === 'usdt' && txnNo.trim() === '')
            }
            onClick={() => paying && pay.mutate(paying)}
          >
            {t('Confirm')}
          </Button>
        }
      >
        <div className='space-y-4'>
          {paying?.account_type === 'usdt' && (
            <div className='space-y-2'>
              <Label htmlFor='pay-txn'>{t('Transaction hash')}</Label>
              <Input
                id='pay-txn'
                value={txnNo}
                onChange={(event) => setTxnNo(event.target.value)}
              />
            </div>
          )}
          <div className='space-y-2'>
            <Label htmlFor='pay-proof'>{t('Payment proof')}</Label>
            <Input
              id='pay-proof'
              type='file'
              accept='image/png,image/jpeg,image/webp'
              onChange={(event) => {
                const file = event.target.files?.[0]
                if (file) upload.mutate(file)
              }}
            />
          </div>
        </div>
      </Dialog>
    </div>
  )
}
