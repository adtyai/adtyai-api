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

import { useEffect, useState } from 'react'
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
import { orderStatusLabelKey } from '@/features/system-settings/orders/lib/orders'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useCompleteAdminOrder } from '../hooks/use-orders'
import {
  completeErrorMessage,
  formatUsd,
  orderUserDisplay,
} from '../lib/orders'
import type { TopupOrder } from '../types'

type Props = {
  order: TopupOrder | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: () => void
}

export function ConfirmCompleteDialog({
  order,
  open,
  onOpenChange,
  onDone,
}: Props) {
  const { t } = useTranslation()
  const mut = useCompleteAdminOrder()
  const [localError, setLocalError] = useState<string | null>(null)
  const [tradeNo, setTradeNo] = useState('')
  const busy = mut.isPending

  useEffect(() => {
    if (open) {
      setLocalError(null)
      setTradeNo('')
      mut.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, order?.order_no])

  if (!order) return null

  async function handleConfirm() {
    if (!order) return
    setLocalError(null)
    if (!tradeNo.trim()) {
      setLocalError(t('Gateway trade number is required'))
      return
    }
    try {
      const res = await mut.mutateAsync({
        orderNo: order.order_no,
        tradeNo,
      })
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Complete failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Order completed'))
      onOpenChange(false)
      onDone?.()
    } catch (err) {
      const status = (err as { status?: number; code?: string })?.status
      const code = (err as { status?: number; code?: string })?.code
      const mapped = completeErrorMessage(
        status,
        code,
        getServerErrorMessage(err, t('Complete failed'))
      )
      const msg = t(mapped)
      setLocalError(msg)
      toast.error(msg)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Complete topup order')}</DialogTitle>
          <DialogDescription>
            {t(
              'Root only. Manual complete credits paid and immediate bonus for a pending order (same path as payment callback). Enter the gateway trade number as evidence; your own orders and orders that never started a payment are rejected.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-2 text-sm'>
          <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
            <div className='col-span-2'>
              <span className='text-foreground font-medium'>
                {t('Order No.')}:{' '}
              </span>
              <span className='font-mono text-[12px]'>{order.order_no}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Status')}:{' '}
              </span>
              <span>{t(orderStatusLabelKey(order.status))}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>{t('User')}: </span>
              {orderUserDisplay(order)} (ID {order.user_id})
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Paid amount')}:{' '}
              </span>
              {formatUsd(order.paid_amount_usd)}
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Bonus amount')}:{' '}
              </span>
              {formatUsd(order.bonus_amount_usd)}
            </div>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='complete-trade-no'>
              {t('Gateway trade number')}
            </Label>
            <Input
              id='complete-trade-no'
              value={tradeNo}
              disabled={busy}
              placeholder={t('Trade no. from the payment gateway backend')}
              onChange={(e) => setTradeNo(e.target.value)}
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
            disabled={busy}
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={busy || order.status !== 'pending' || !tradeNo.trim()}
            onClick={() => void handleConfirm()}
          >
            {t('Complete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
