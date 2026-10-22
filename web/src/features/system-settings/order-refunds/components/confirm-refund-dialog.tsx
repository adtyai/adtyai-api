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

import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  orderStatusLabelKey,
  refundStatusLabelKey,
} from '@/features/system-settings/orders/lib/orders'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { previewTopupRefund, type RefundClawPreview } from '../api'
import { useAdminRefundTopupOrder } from '../hooks/use-order-refund'
import { newRefundRequestId, refundHintUsd } from '../lib/refundable'
import type { RefundTopupOrderResult, TopupOrder } from '../types'

type Props = {
  order: TopupOrder | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: (result: RefundTopupOrderResult) => void
}

function fmtUsd(n: number): string {
  return formatBillingCurrencyFromUSD(n, {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })
}

export function ConfirmRefundDialog({
  order,
  open,
  onOpenChange,
  onDone,
}: Props) {
  const { t } = useTranslation()
  const mut = useAdminRefundTopupOrder()
  const [amountStr, setAmountStr] = useState('')
  const [reason, setReason] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)
  const [preview, setPreview] = useState<RefundClawPreview | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [amountConfirm, setAmountConfirm] = useState('')

  const hint = useMemo(() => (order ? refundHintUsd(order) : 0), [order])

  useEffect(() => {
    if (open) {
      setAmountStr(hint > 0 ? hint.toFixed(2) : '')
      setReason('')
      setLocalError(null)
      setPreview(null)
      setPreviewError(null)
      setAmountConfirm('')
      mut.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, order?.order_no])

  useEffect(() => {
    if (!open || !order) return
    const amount = Number(amountStr)
    if (!Number.isFinite(amount) || amount <= 0) {
      setPreview(null)
      setPreviewError(null)
      return
    }
    let cancelled = false
    const timer = window.setTimeout(() => {
      void previewTopupRefund(order.order_no, amount)
        .then((data) => {
          if (cancelled) return
          setPreview(data)
          setPreviewError(null)
        })
        .catch((err) => {
          if (cancelled) return
          setPreview(null)
          setPreviewError(
            getServerErrorMessage(err, t('Refund preview failed'))
          )
        })
    }, 300)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [open, order, amountStr, t])

  if (!order) return null

  async function handleConfirm() {
    if (!order) return
    setLocalError(null)
    const amount = Number(amountStr)
    if (!Number.isFinite(amount) || amount <= 0) {
      const msg = t('Amount must be greater than 0')
      setLocalError(msg)
      return
    }
    if (!reason.trim()) {
      const msg = t('Refund reason is required')
      setLocalError(msg)
      return
    }
    if (!preview) {
      const msg = t('Confirm stays disabled until the clawback preview loads.')
      setLocalError(msg)
      return
    }
    if (amount >= 100 && Math.abs(Number(amountConfirm) - amount) > 1e-6) {
      const msg = t('Retype the refund amount to confirm')
      setLocalError(msg)
      return
    }
    const requestId = newRefundRequestId()
    try {
      const res = await mut.mutateAsync({
        orderNo: order.order_no,
        amount,
        reason: reason.trim(),
        request_id: requestId,
        amount_confirm: amount >= 100 ? Number(amountConfirm) : undefined,
        preview: {
          amount_usd: preview.amount_usd,
          paid_claw_usd: preview.paid_claw_usd,
          bonus_claw_usd: preview.bonus_claw_usd,
          bonus_shortfall_usd: preview.bonus_shortfall_usd,
          aff_claw_usd: preview.aff_claw_usd,
          aff_action: preview.aff_action,
        },
      })
      if (!res.success || !res.data) {
        const msg = getServerErrorMessage(res, t('Refund failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Refund recorded'))
      onOpenChange(false)
      onDone?.(res.data)
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Refund failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Refund order')}</DialogTitle>
          <DialogDescription>
            {t(
              'Ledger-only refund: does not call the payment channel. Requires a fresh request id. Submit takes effect immediately.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-3 text-sm'>
          <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
            <div className='col-span-2 break-all'>
              {t('Order No.')}:{' '}
              <span className='font-mono'>{order.order_no}</span>
            </div>
            <div>
              {t('User')}: {order.user_id}
            </div>
            <div>
              {t('Status')}: <span>{t(orderStatusLabelKey(order.status))}</span>
            </div>
            <div>
              {t('Refund status')}:{' '}
              <span>
                {t(refundStatusLabelKey(order.refund_status || 'none'))}
              </span>
            </div>
            <div>
              {t('Already refunded')}:{' '}
              {fmtUsd(Number(order.refunded_amount) || 0)}
            </div>
            <div className='col-span-2'>
              {t('Refundable hint')}: {fmtUsd(hint)}{' '}
              <span className='text-muted-foreground'>
                ({t('system validates final amount')})
              </span>
            </div>
          </div>
          {preview ? (
            <div className='bg-muted/40 grid gap-1 rounded-lg px-3 py-2 text-xs'>
              <div className='font-medium'>{t('Refund preview')}</div>
              <div>
                {t('Paid balance clawback')}: {fmtUsd(preview.paid_claw_usd)}
              </div>
              <div>
                {t('Bonus clawback')}: {fmtUsd(preview.bonus_claw_usd)}
              </div>
              <div>
                {t('Bonus not clawed')}: {fmtUsd(preview.bonus_shortfall_usd)}
              </div>
              <div>
                {preview.aff_action === 'void'
                  ? t('Invite reward voided')
                  : t('Invite reward clawback')}
                : {fmtUsd(preview.aff_claw_usd)}
              </div>
              <div className='text-muted-foreground'>
                {t(
                  'Confirm stays disabled until the clawback preview loads.'
                )}
              </div>
            </div>
          ) : null}
          {previewError ? (
            <div className='text-destructive text-xs'>{previewError}</div>
          ) : null}

          <div className='grid gap-1.5'>
            <Label htmlFor='refund-amount'>{t('Refund amount (USD)')}</Label>
            <Input
              id='refund-amount'
              type='number'
              min={0}
              step='0.01'
              value={amountStr}
              onChange={(e) => {
                setAmountStr(e.target.value)
              }}
              disabled={mut.isPending}
            />
          </div>
          {Number(amountStr) >= 100 ? (
            <div className='grid gap-1.5'>
              <Label htmlFor='refund-amount-confirm'>
                {t('Retype the refund amount to confirm')}
              </Label>
              <Input
                id='refund-amount-confirm'
                type='number'
                min={0}
                step='0.01'
                value={amountConfirm}
                onChange={(e) => setAmountConfirm(e.target.value)}
                disabled={mut.isPending}
              />
            </div>
          ) : null}
          <div className='grid gap-1.5'>
            <Label htmlFor='refund-reason'>{t('Reason')}</Label>
            <Textarea
              id='refund-reason'
              value={reason}
              onChange={(e) => {
                setReason(e.target.value)
              }}
              rows={3}
              disabled={mut.isPending}
              placeholder={t('Refund reason (required)')}
            />
          </div>

          {localError ? (
            <div className='text-destructive border-destructive/30 bg-destructive/5 rounded-lg border px-3 py-2 text-xs'>
              {localError}
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={mut.isPending}
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            variant='destructive'
            disabled={mut.isPending || !preview || !reason.trim()}
            onClick={() => void handleConfirm()}
          >
            {t('Refund')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
