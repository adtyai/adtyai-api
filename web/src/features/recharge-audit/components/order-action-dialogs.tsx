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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  completeTopupManually,
  previewTopupRefund,
  queryTopupPayment,
  refundTopup,
  type RechargeAuditOrder,
  type TopupRefundResult,
} from '../api'

type OrderActionProps = {
  order: RechargeAuditOrder | null
  onOpenChange: (open: boolean) => void
  onDone: () => void
}

export function QueryPaymentDialog(props: OrderActionProps) {
  const { t } = useTranslation()
  const mutation = useMutation({
    meta: { errorToast: false },
    mutationFn: () => {
      if (
        !props.order?.actions.query ||
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.FINANCE_MANUAL_CREDIT
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return queryTopupPayment(props.order.trade_no)
    },
    onSuccess: () => props.onDone(),
  })
  const resetQuery = mutation.reset
  useEffect(() => resetQuery(), [props.order?.trade_no, resetQuery])
  const completed = mutation.isSuccess && mutation.data.status === 'success'
  return (
    <ConfirmDialog
      open={props.order !== null}
      onOpenChange={props.onOpenChange}
      title={t('Query payment')}
      desc={t(
        'Query the original payment provider. A confirmed payment is credited using the saved order details.'
      )}
      confirmText={t('Query payment')}
      handleConfirm={() => mutation.mutate()}
      isLoading={mutation.isPending}
      disabled={completed}
    >
      <code className='text-xs break-all'>{props.order?.trade_no}</code>
      {mutation.isError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {getServerErrorMessage(mutation.error)}
          </AlertDescription>
        </Alert>
      )}
      {mutation.isSuccess && (
        <Alert>
          <AlertDescription>
            {completed ? t('Order completed') : t('No confirmed payment found')}
          </AlertDescription>
        </Alert>
      )}
    </ConfirmDialog>
  )
}

/** Manual completion after checking the payment at the provider. */
export function ManualCompleteDialog(props: OrderActionProps) {
  const { t } = useTranslation()
  const [proof, setProof] = useState('')
  const [note, setNote] = useState('')
  useEffect(() => {
    setProof('')
    setNote('')
  }, [props.order])
  const mutation = useMutation({
    mutationFn: () =>
      completeTopupManually(
        props.order?.trade_no ?? '',
        proof.trim(),
        note.trim()
      ),
    onSuccess: () => {
      toast.success(t('Order completed'))
      props.onDone()
      props.onOpenChange(false)
    },
    onError: (error) => handleServerError(error),
  })
  const ready = proof.trim() !== '' && note.trim() !== ''
  return (
    <Dialog
      open={props.order !== null}
      onOpenChange={props.onOpenChange}
      title={t('Complete order manually')}
      description={t(
        'Check the payment in the provider console first. The order is credited with its face value and bonus.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!ready || mutation.isPending}
          >
            {t('Complete')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label htmlFor='manual-proof'>
          {t('Proof (transaction hash or platform order number)')}
        </Label>
        <Input
          id='manual-proof'
          value={proof}
          onChange={(e) => setProof(e.target.value)}
        />
      </div>
      <div className='space-y-2'>
        <Label htmlFor='manual-note'>{t('Note')}</Label>
        <Textarea
          id='manual-note'
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={3}
        />
      </div>
    </Dialog>
  )
}

/**
 * Refund: takes back what is left of this order from the user's current
 * balance. The bonus is not refunded, only the unspent principal. The backend
 * computes the amount; the money itself is returned offline.
 */
export function RefundDialog(props: OrderActionProps) {
  const { t } = useTranslation()
  const [reason, setReason] = useState('')
  const [refundUsd, setRefundUsd] = useState('')
  const [idempotencyKey, setIdempotencyKey] = useState('')
  const [result, setResult] = useState<TopupRefundResult | null>(null)
  const typedAmount = refundUsd.trim()
  const typedReady = typedAmount === '' || /^\d+(\.\d{1,8})?$/.test(typedAmount)
  useEffect(() => {
    setReason('')
    setRefundUsd('')
    setResult(null)
    setIdempotencyKey(crypto.randomUUID())
  }, [props.order])
  const preview = useQuery({
    queryKey: [
      'topup-refund-preview',
      props.order?.trade_no,
      idempotencyKey,
      typedReady ? typedAmount : '',
    ],
    queryFn: () =>
      previewTopupRefund(
        props.order?.trade_no ?? '',
        typedAmount || undefined
      ),
    enabled: props.order !== null && result === null && typedReady,
  })
  const mutation = useMutation({
    mutationFn: () =>
      refundTopup(
        props.order?.trade_no ?? '',
        reason.trim(),
        idempotencyKey,
        preview.data?.refund_quota ?? 0,
        typedAmount || undefined
      ),
    onSuccess: (data) => {
      toast.success(t('Order refunded'))
      setResult(data)
      props.onDone()
    },
    onError: (error) => handleServerError(error),
  })
  if (result) {
    return (
      <Dialog
        open={props.order !== null}
        onOpenChange={props.onOpenChange}
        title={t('Order refunded')}
        contentHeight='auto'
        bodyClassName='space-y-2'
        footer={
          <Button onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
        }
      >
        <p>
          {t(
            'Took back {{quota}} of credit. Refund {{amount}} {{currency}} to the user offline.',
            {
              quota: formatQuota(result.deducted_quota),
              amount: result.refund_amount,
              currency: result.currency,
            }
          )}
        </p>
        {result.refund_quota === 0 && (
          <p className='text-muted-foreground text-sm'>
            {t(
              'Nothing to refund: the credit left does not exceed the bonus of this order.'
            )}
          </p>
        )}
      </Dialog>
    )
  }
  return (
    <Dialog
      open={props.order !== null}
      onOpenChange={props.onOpenChange}
      title={t('Refund order')}
      description={t(
        "Takes back what is left of this order's credit from the user's current balance. The bonus is not refunded, only the unspent principal. Return the money to the user offline."
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => mutation.mutate()}
            disabled={
              reason.trim() === '' ||
              !typedReady ||
              mutation.isPending ||
              preview.data === undefined
            }
          >
            {t('Refund')}
          </Button>
        </>
      }
    >
      {preview.data && typedAmount === '' && (
        <p className='text-sm'>
          {t(
            'Refund {{amount}} {{currency}}. This takes back {{quota}} of credit.',
            {
              amount: preview.data.refund_amount,
              currency: preview.data.currency,
              quota: formatQuota(preview.data.deducted_quota),
            }
          )}
        </p>
      )}
      {preview.data?.facts && (
        <dl className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm'>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Site ID')}</dt>
            <dd className='text-right tabular-nums'>{preview.data.facts.site_id}</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Customer ID')}</dt>
            <dd className='text-right tabular-nums'>{preview.data.facts.user_id}</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Original principal')}</dt>
            <dd className='text-right tabular-nums'>{preview.data.facts.face_usd} USD</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Paid amount')}</dt>
            <dd className='text-right tabular-nums'>
              {preview.data.facts.paid_amount} {preview.data.facts.paid_currency}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Order rate')}</dt>
            <dd className='text-right tabular-nums'>{preview.data.facts.order_rate}</dd>
          </div>
          {preview.data.facts.principal_consumed_usd && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Consumed principal')}</dt>
              <dd className='text-right tabular-nums'>
                {preview.data.facts.principal_consumed_usd} USD
              </dd>
            </div>
          )}
          {preview.data.facts.principal_remaining_usd && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Remaining principal')}</dt>
              <dd className='text-right tabular-nums'>
                {preview.data.facts.principal_remaining_usd} USD
              </dd>
            </div>
          )}
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('In-flight precharge')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(preview.data.facts.pending_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Refund occupancy')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(preview.data.facts.refund_occupied_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Bonus')}</dt>
            <dd className='text-right tabular-nums'>{preview.data.facts.bonus_usd} USD</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Bonus used')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(preview.data.facts.bonus_used_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Bonus remaining')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(preview.data.facts.bonus_remaining_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Cumulative refund')}</dt>
            <dd className='text-right tabular-nums'>
              {preview.data.facts.cumulative_refund_usd} USD
            </dd>
          </div>
          {preview.data.facts.foreign_reference && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Foreign reference')}</dt>
              <dd className='text-right tabular-nums'>
                {preview.data.facts.foreign_reference}{' '}
                {preview.data.facts.paid_currency}
              </dd>
            </div>
          )}
          {preview.data.facts.refund_proof && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Proof')}</dt>
              <dd className='text-right break-all'>{preview.data.facts.refund_proof}</dd>
            </div>
          )}
        </dl>
      )}
      <div className='space-y-1'>
        <Label htmlFor='refund-usd'>{t('Refund amount in USD')}</Label>
        <Input
          id='refund-usd'
          value={refundUsd}
          inputMode='decimal'
          autoComplete='off'
          onChange={(event) => setRefundUsd(event.target.value)}
        />
      </div>
      {preview.data && (
        <div className='space-y-1 text-sm'>
          {preview.data.commission_issued_usd && (
            <p className='tabular-nums'>
              {t('Issued commission')}: {preview.data.commission_issued_usd} USD
            </p>
          )}
          {preview.data.commission_withdrawn_usd && (
            <p className='tabular-nums'>
              {t('Withdrawn commission')}: {preview.data.commission_withdrawn_usd}{' '}
              USD
            </p>
          )}
          {preview.data.commission_clawback_usd && (
            <p className='tabular-nums'>
              {t('Expected commission clawback')}:{' '}
              {preview.data.commission_clawback_usd} USD
            </p>
          )}
        </div>
      )}
      <div className='space-y-2'>
        <Label htmlFor='refund-reason'>{t('Reason (required)')}</Label>
        <Textarea
          id='refund-reason'
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={3}
        />
      </div>
    </Dialog>
  )
}
