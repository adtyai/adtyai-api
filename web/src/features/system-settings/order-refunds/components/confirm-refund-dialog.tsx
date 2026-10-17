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

  const hint = useMemo(() => (order ? refundHintUsd(order) : 0), [order])

  useEffect(() => {
    if (open) {
      setAmountStr(hint > 0 ? hint.toFixed(2) : '')
      setReason('')
      setLocalError(null)
      mut.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, order?.order_no])

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
    const requestId = newRefundRequestId()
    try {
      const res = await mut.mutateAsync({
        orderNo: order.order_no,
        amount,
        reason: reason.trim(),
        request_id: requestId,
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
            disabled={mut.isPending}
            onClick={() => void handleConfirm()}
          >
            {t('Refund')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
