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

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  completeTopupManually,
  previewTopupRefund,
  refundTopup,
  type RechargeAuditOrder,
  type TopupRefundResult,
} from '../api'

type OrderActionProps = {
  order: RechargeAuditOrder | null
  onOpenChange: (open: boolean) => void
  onDone: () => void
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
        'Check the payment in the BEpusdt or payment-channel console first. The order is credited with its face value and bonus.'
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
  const [idempotencyKey, setIdempotencyKey] = useState('')
  const [result, setResult] = useState<TopupRefundResult | null>(null)
  useEffect(() => {
    setReason('')
    setResult(null)
    setIdempotencyKey(crypto.randomUUID())
  }, [props.order])
  const preview = useQuery({
    queryKey: ['topup-refund-preview', props.order?.trade_no, idempotencyKey],
    queryFn: () => previewTopupRefund(props.order?.trade_no ?? ''),
    enabled: props.order !== null && result === null,
  })
  const mutation = useMutation({
    mutationFn: () =>
      refundTopup(
        props.order?.trade_no ?? '',
        reason.trim(),
        idempotencyKey,
        preview.data?.refund_quota ?? 0
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
              mutation.isPending ||
              preview.data === undefined
            }
          >
            {t('Refund')}
          </Button>
        </>
      }
    >
      {preview.data && (
        <p>
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
