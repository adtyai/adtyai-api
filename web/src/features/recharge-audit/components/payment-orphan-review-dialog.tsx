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
import { useMutation } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { DraftDiscardDialog } from '@/features/system-settings/components/draft-discard-dialog'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  resolvePaymentOrphan,
  type PaymentOrphanDismissalReason,
  type PaymentOrphanEvent,
  type PaymentOrphanReviewRequest,
  type PaymentOrphanReviewStatus,
} from '../api'

const outcomes = {
  pending_review: 'Pending review',
  refunded: 'Reconciled refund',
  never_credited: 'Never credited',
  dismissed: 'Dismiss',
} as const

const dismissalReasons = {
  not_this_order: 'This reversal belongs to another order',
  reversal_canceled: 'The provider canceled this reversal',
} as const

/** Records provider evidence and existing refund facts without moving money. */
export function PaymentOrphanReviewDialog(props: {
  event: PaymentOrphanEvent
  canReview: boolean
  onClose: () => void
  onDone: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const review = props.event.review
  const initial = {
    status: 'pending_review' as PaymentOrphanReviewStatus,
    amount: review?.external_amount ?? '0',
    currency: review?.external_currency ?? '',
    proof: review?.proof ?? '',
    note: review?.note ?? '',
    dismissal: '' as PaymentOrphanDismissalReason | '',
  }
  const [draft, setDraft] = useState(initial)
  const [discardOpen, setDiscardOpen] = useState(false)
  const editable = props.canReview && props.event.status === 'pending_review'
  const dirty = Object.keys(initial).some(
    (key) =>
      draft[key as keyof typeof draft] !== initial[key as keyof typeof initial]
  )
  const mutation = useMutation({
    mutationFn: (request: PaymentOrphanReviewRequest) =>
      resolvePaymentOrphan(props.event.id, request),
    onSuccess: () => {
      toast.success(t('Review saved'))
      props.onDone()
      props.onClose()
    },
    onError: (error) => handleServerError(error),
  })
  const close = () => {
    if (mutation.isPending) return
    if (editable && dirty) setDiscardOpen(true)
    else props.onClose()
  }
  const amount = draft.amount.trim()
  const decimal = /^[0-9]{1,12}(?:\.[0-9]{1,8})?$/.test(amount)
  const positive = decimal && /[1-9]/.test(amount)
  const dismissed = draft.status === 'dismissed'
  const validAmount = decimal && (draft.status === 'pending_review' || positive)
  const ready =
    editable &&
    props.event.reference_id.trim() !== '' &&
    draft.proof.trim() !== '' &&
    draft.proof.trim().length <= 512 &&
    draft.note.trim() !== '' &&
    draft.note.trim().length <= 2000 &&
    (dismissed
      ? draft.dismissal !== ''
      : validAmount &&
        /^[A-Z]{3,16}$/.test(draft.currency.trim().toUpperCase()))
  const save = () => {
    if (!ready || mutation.isPending) return
    mutation.mutate({
      status: draft.status,
      trade_no: props.event.reference_id,
      external_amount: dismissed ? '0' : amount,
      external_currency: dismissed ? '' : draft.currency.trim().toUpperCase(),
      proof: draft.proof.trim(),
      note: draft.note.trim(),
      ...(dismissed && draft.dismissal
        ? { dismissal_reason: draft.dismissal }
        : {}),
    })
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) close()
        }}
        title={t('Review payment event')}
        description={t(
          'Record verified provider evidence. Use Refund on the original order to take back wallet credit; saving this review does not deduct credit.'
        )}
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              variant='outline'
              disabled={mutation.isPending}
              onClick={close}
            >
              {t(editable ? 'Cancel' : 'Close')}
            </Button>
            {editable && (
              <Button disabled={!ready || mutation.isPending} onClick={save}>
                {t('Save review')}
              </Button>
            )}
          </>
        }
      >
        <div className='space-y-2'>
          <Label htmlFor={`${id}-order`}>{t('Order number')}</Label>
          <Input id={`${id}-order`} value={props.event.reference_id} readOnly />
          {!props.event.reference_id.trim() && (
            <p role='note' className='text-muted-foreground text-sm'>
              {t(
                'Wait for this payment event to be linked to its original order before reviewing it.'
              )}
            </p>
          )}
        </div>
        {review ? (
          <section
            aria-label={t('Latest recorded review')}
            className='bg-muted/30 space-y-3 rounded-md border p-3 text-sm'
          >
            <h3 className='font-medium'>{t('Latest recorded review')}</h3>
            <dl className='grid gap-x-4 gap-y-2 sm:grid-cols-2'>
              <dt className='text-muted-foreground'>{t('Review outcome')}</dt>
              <dd>{t(outcomes[review.status])}</dd>
              <dt className='text-muted-foreground'>
                {t('Cumulative external reversal amount')}
              </dt>
              <dd>
                {review.external_amount} {review.external_currency}
              </dd>
              <dt className='text-muted-foreground'>
                {t('Proof (transaction hash or platform order number)')}
              </dt>
              <dd className='break-all'>{review.proof}</dd>
              <dt className='text-muted-foreground'>{t('Note')}</dt>
              <dd className='break-words whitespace-pre-wrap'>{review.note}</dd>
              <dt className='text-muted-foreground'>{t('Time')}</dt>
              <dd>{formatTimestamp(review.created_at)}</dd>
              <dt className='text-muted-foreground'>{t('Operator ID')}</dt>
              <dd>{review.operator_id}</dd>
              {review.dismissal_reason && (
                <>
                  <dt className='text-muted-foreground'>
                    {t('Dismissal reason')}
                  </dt>
                  <dd>{t(dismissalReasons[review.dismissal_reason])}</dd>
                </>
              )}
              {review.refund_ledger_id > 0 && (
                <>
                  <dt className='text-muted-foreground'>
                    {t('Linked refund ledger')}
                  </dt>
                  <dd>{review.refund_ledger_id}</dd>
                  <dt className='text-muted-foreground'>
                    {t('Wallet credit taken back')}
                  </dt>
                  <dd>{formatQuota(review.deducted_quota)}</dd>
                  <dt className='text-muted-foreground'>
                    {t('Principal credit taken back')}
                  </dt>
                  <dd>{formatQuota(review.refund_quota)}</dd>
                  <dt className='text-muted-foreground'>
                    {t('Principal cash coverage')}
                  </dt>
                  <dd>
                    {review.refund_amount} {review.paid_currency}
                  </dd>
                  <dt className='text-muted-foreground'>
                    {t('Uncovered external reversal amount')}
                  </dt>
                  <dd>
                    {review.exposure_amount} {review.paid_currency}
                  </dd>
                </>
              )}
            </dl>
            {!review.refund_ledger_id && (
              <p className='text-muted-foreground'>
                {t(
                  'No linked refund; cash coverage and exposure have not been calculated.'
                )}
              </p>
            )}
          </section>
        ) : (
          <p className='text-muted-foreground text-sm'>
            {t('No review has been recorded yet.')}
          </p>
        )}
        {editable && (
          <fieldset disabled={mutation.isPending} className='space-y-4'>
            <div className='space-y-2'>
              <Label htmlFor={`${id}-outcome`}>{t('Review outcome')}</Label>
              <NativeSelect
                id={`${id}-outcome`}
                className='w-full'
                value={draft.status}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    status: event.target.value as PaymentOrphanReviewStatus,
                  })
                }
              >
                {Object.entries(outcomes).map(([status, label]) => (
                  <option key={status} value={status}>
                    {t(label)}
                  </option>
                ))}
              </NativeSelect>
            </div>
            {dismissed ? (
              <>
                <p role='note' className='text-destructive text-sm'>
                  {t(
                    'Dismissing this event removes the block on late payment credit.'
                  )}
                </p>
                <div className='space-y-2'>
                  <Label htmlFor={`${id}-dismissal`}>
                    {t('Dismissal reason')}
                  </Label>
                  <NativeSelect
                    id={`${id}-dismissal`}
                    className='w-full'
                    value={draft.dismissal}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        dismissal: event.target.value as
                          | PaymentOrphanDismissalReason
                          | '',
                      })
                    }
                  >
                    <option value=''>{t('Select a reason')}</option>
                    {Object.entries(dismissalReasons).map(([reason, label]) => (
                      <option key={reason} value={reason}>
                        {t(label)}
                      </option>
                    ))}
                  </NativeSelect>
                </div>
              </>
            ) : (
              <>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Keep partial reversals pending for reconciliation. Reconciled refunds require a matching refund ledger and a full external reversal.'
                  )}
                </p>
                <div className='grid gap-4 sm:grid-cols-2'>
                  <div className='space-y-2'>
                    <Label htmlFor={`${id}-amount`}>
                      {t('Cumulative external reversal amount')}
                    </Label>
                    <Input
                      id={`${id}-amount`}
                      inputMode='decimal'
                      value={draft.amount}
                      aria-invalid={!validAmount}
                      aria-describedby={`${id}-amount-help`}
                      onChange={(event) =>
                        setDraft({ ...draft, amount: event.target.value })
                      }
                    />
                    <p
                      id={`${id}-amount-help`}
                      className='text-muted-foreground text-xs'
                    >
                      {t('Enter up to 12 whole digits and 8 decimal places.')}
                    </p>
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor={`${id}-currency`}>
                      {t('External reversal currency')}
                    </Label>
                    <Input
                      id={`${id}-currency`}
                      value={draft.currency}
                      maxLength={16}
                      onChange={(event) =>
                        setDraft({ ...draft, currency: event.target.value })
                      }
                    />
                  </div>
                </div>
              </>
            )}
            <div className='space-y-2'>
              <Label htmlFor={`${id}-proof`}>
                {t('Proof (transaction hash or platform order number)')}
              </Label>
              <Input
                id={`${id}-proof`}
                value={draft.proof}
                maxLength={512}
                onChange={(event) =>
                  setDraft({ ...draft, proof: event.target.value })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`${id}-note`}>{t('Note')}</Label>
              <Textarea
                id={`${id}-note`}
                value={draft.note}
                rows={3}
                maxLength={2000}
                onChange={(event) =>
                  setDraft({ ...draft, note: event.target.value })
                }
              />
            </div>
          </fieldset>
        )}
      </Dialog>
      <DraftDiscardDialog
        open={discardOpen}
        onOpenChange={setDiscardOpen}
        onDiscard={props.onClose}
      />
    </>
  )
}
