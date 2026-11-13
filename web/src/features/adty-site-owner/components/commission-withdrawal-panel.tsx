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
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usdtNetworkLabel } from '@/features/referral/lib/address'
import {
  WITHDRAWAL_STATUS_LABELS,
  WITHDRAWAL_TARGET_LABELS,
} from '@/features/referral/lib/labels'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getOwnerCommissionWithdrawals,
  markOwnerCommissionWithdrawalPaid,
  reviewOwnerCommissionWithdrawal,
  siteOwnerKeys,
  type SiteInviteCommissionWithdrawal,
} from '../api'

function methodLabel(method: string): string {
  if (method in WITHDRAWAL_TARGET_LABELS) {
    return WITHDRAWAL_TARGET_LABELS[method as keyof typeof WITHDRAWAL_TARGET_LABELS]
  }
  return method
}

function statusLabel(status: string): string {
  if (status in WITHDRAWAL_STATUS_LABELS) {
    return WITHDRAWAL_STATUS_LABELS[status as keyof typeof WITHDRAWAL_STATUS_LABELS]
  }
  return status
}

function Payee(props: { row: SiteInviteCommissionWithdrawal }) {
  const { t } = useTranslation()
  const row = props.row
  if (row.method === 'usdt') {
    return (
      <div className='flex flex-col gap-1'>
        <span>{usdtNetworkLabel(row.network)}</span>
        <div className='flex items-center gap-1'>
          <span className='font-mono text-sm break-all'>{row.account_no}</span>
          <CopyButton value={row.account_no} size='icon' aria-label={t('Copy address')} />
        </div>
      </div>
    )
  }
  if (row.method === 'balance') return <span>{t('Account balance')}</span>
  return (
    <div className='flex items-center gap-2'>
      {row.qr_image_url !== '' && (
        <a href={row.qr_image_url} target='_blank' rel='noreferrer'>
          <img
            src={row.qr_image_url}
            alt={t('Payment QR code')}
            className='h-16 w-16 rounded-lg border object-contain'
          />
        </a>
      )}
      <div className='flex flex-col'>
        <span>{t(methodLabel(row.method))}</span>
        <span>{row.account_name}</span>
      </div>
    </div>
  )
}

function ReviewRow(props: { row: SiteInviteCommissionWithdrawal; onDone: () => void }) {
  const { t } = useTranslation()
  const row = props.row
  const [reason, setReason] = useState('')
  const [txId, setTxId] = useState('')
  const [proof, setProof] = useState('')
  const review = useMutation({
    mutationFn: (approve: boolean) =>
      reviewOwnerCommissionWithdrawal(row.id, approve, reason),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      setReason('')
      props.onDone()
    },
    onError: (err) => handleServerError(err),
  })
  const pay = useMutation({
    mutationFn: () => markOwnerCommissionWithdrawalPaid(row.id, txId.trim(), proof.trim()),
    onSuccess: () => {
      toast.success(t('Marked as paid'))
      setTxId('')
      setProof('')
      props.onDone()
    },
    onError: (err) => handleServerError(err),
  })
  return (
    <li className='flex flex-col gap-2 text-sm'>
      <p className='tabular-nums'>
        {row.username || row.user_id} · {row.amount_usd} USD · {t(methodLabel(row.method))} ·{' '}
        {t(statusLabel(row.status))}
        {row.payout_amount !== '' && ` · ${row.payout_amount} ${row.payout_currency}`}
      </p>
      <Payee row={row} />
      {row.status === 'pending' && row.method !== 'balance' && (
        <div className='flex flex-col gap-2'>
          <Label htmlFor={`commission-reject-${row.id}`}>{t('Reason')}</Label>
          <Input
            id={`commission-reject-${row.id}`}
            value={reason}
            autoComplete='off'
            onChange={(event) => setReason(event.target.value)}
          />
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={review.isPending}
              onClick={() => review.mutate(true)}
            >
              {t('Approve')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={review.isPending}
              onClick={() => review.mutate(false)}
            >
              {t('Reject')}
            </Button>
          </div>
        </div>
      )}
      {row.status === 'approved' && (
        <form
          className='flex flex-col gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            pay.mutate()
          }}
        >
          {row.method === 'usdt' && (
            <div className='flex flex-col gap-2'>
              <Label htmlFor={`commission-tx-${row.id}`}>{t('Transaction hash')}</Label>
              <Input
                id={`commission-tx-${row.id}`}
                value={txId}
                autoComplete='off'
                onChange={(event) => setTxId(event.target.value)}
                required
              />
            </div>
          )}
          <div className='flex flex-col gap-2'>
            <Label htmlFor={`commission-proof-${row.id}`}>{t('Proof note')}</Label>
            <Input
              id={`commission-proof-${row.id}`}
              value={proof}
              autoComplete='off'
              onChange={(event) => setProof(event.target.value)}
            />
          </div>
          <div className='flex flex-col gap-2'>
            <Label htmlFor={`commission-fail-${row.id}`}>{t('Reason')}</Label>
            <Input
              id={`commission-fail-${row.id}`}
              value={reason}
              autoComplete='off'
              onChange={(event) => setReason(event.target.value)}
            />
          </div>
          <div className='flex flex-wrap gap-2'>
            <Button type='submit' variant='outline' disabled={pay.isPending}>
              {t('Mark as paid')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={review.isPending}
              onClick={() => review.mutate(false)}
            >
              {t('Reject')}
            </Button>
          </div>
        </form>
      )}
    </li>
  )
}

/** The site owner reviews customer invite-commission withdrawals. Root does not. */
export function CommissionWithdrawalPanel() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const list = useQuery({
    queryKey: siteOwnerKeys.list('commission-withdrawals', {}),
    queryFn: getOwnerCommissionWithdrawals,
  })
  const refresh = () =>
    void client.invalidateQueries({ queryKey: siteOwnerKeys.all })

  return (
    <section className='flex flex-col gap-4' aria-label={t('Invite commission withdrawal')}>
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('Invite commission withdrawal')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t('The site owner reviews cash withdrawals of invite commission.')}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'A transfer to balance deducts the site hold. External payout releases it after payment is complete.'
          )}
        </p>
      </div>
      {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
      {list.isSuccess && list.data.length === 0 && (
        <p className='text-muted-foreground text-sm'>{t('No withdrawals yet')}</p>
      )}
      <ul className='flex flex-col gap-4'>
        {(list.data ?? []).map((row) => (
          <ReviewRow key={row.id} row={row} onDone={refresh} />
        ))}
      </ul>
    </section>
  )
}
