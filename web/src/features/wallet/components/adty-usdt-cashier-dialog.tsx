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
import { AlertTriangle, Clock, Copy, ExternalLink, Loader2 } from 'lucide-react'
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { cn } from '@/lib/utils'

import { getAdtyTopupOrder } from '../api'
import {
  formatCountdown,
  formatUsdAmount,
  formatUsdtPayAmount,
  remainingSecondsUntil,
} from '../lib/adty-topup'
import type { AdtyTopupOrder, AdtyUsdtCashier } from '../types'

const POLL_MS = 4000

export interface AdtyUsdtCashierDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: AdtyTopupOrder | null
  usdt: AdtyUsdtCashier | null
  /** Called when order status becomes paid (refresh balance). */
  onPaid?: () => void
}

export function AdtyUsdtCashierDialog({
  open,
  onOpenChange,
  order,
  usdt,
  onPaid,
}: AdtyUsdtCashierDialogProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const [nowMs, setNowMs] = useState(() => Date.now())
  const [pollStatus, setPollStatus] = useState<string | undefined>(
    order?.status
  )
  const [polling, setPolling] = useState(false)
  const stoppedRef = useRef(false)
  const paidFiredRef = useRef(false)
  const expireToastFiredRef = useRef(false)

  const orderNo =
    (order?.order_no as string | undefined) ||
    (typeof order?.['order_no'] === 'string' ? (order['order_no'] as string) : '')

  const amountStr = usdt ? formatUsdtPayAmount(usdt) : ''
  const expireAt = usdt?.expire_at ?? 0
  const remaining = remainingSecondsUntil(expireAt, nowMs)
  const expired = expireAt > 0 && remaining <= 0

  // Tick countdown every second while open
  useEffect(() => {
    if (!open) return
    setNowMs(Date.now())
    const id = window.setInterval(() => setNowMs(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [open])

  // Reset poll state when a new cashier opens
  useEffect(() => {
    if (!open) return
    stoppedRef.current = false
    paidFiredRef.current = false
    expireToastFiredRef.current = false
    setPollStatus(order?.status)
  }, [open, order?.order_no, order?.status])

  const stopPoll = useCallback(() => {
    stoppedRef.current = true
    setPolling(false)
  }, [])

  // Poll order status
  useEffect(() => {
    if (!open || !orderNo || !usdt) return
    if (pollStatus === 'paid' || pollStatus === 'failed' || pollStatus === 'need_manual') {
      return
    }
    if (expired) {
      setPollStatus((s) => (s === 'pending' || !s ? 'failed' : s))
      stopPoll()
      if (!expireToastFiredRef.current) {
        expireToastFiredRef.current = true
        toast.error(t('USDT payment failed or expired. Do not transfer again.'))
      }
      return
    }

    let cancelled = false
    setPolling(true)

    const tick = async () => {
      if (cancelled || stoppedRef.current) return
      try {
        const res = await getAdtyTopupOrder(orderNo)
        if (cancelled || stoppedRef.current) return
        const status = res?.data?.status
        if (!status) return
        setPollStatus(status)
        if (status === 'paid') {
          stopPoll()
          if (!paidFiredRef.current) {
            paidFiredRef.current = true
            toast.success(t('USDT payment confirmed. Balance updated.'))
            onPaid?.()
            onOpenChange(false)
          }
          return
        }
        if (status === 'need_manual') {
          stopPoll()
          toast.warning(
            t(
              'Underpayment or timeout — order needs manual review. Contact support with TxID.'
            )
          )
          return
        }
        if (status === 'failed') {
          stopPoll()
          toast.error(t('USDT payment failed or expired. Do not transfer again.'))
          return
        }
      } catch {
        // keep polling on transient errors
      }
    }

    void tick()
    const id = window.setInterval(() => void tick(), POLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [
    open,
    orderNo,
    usdt,
    pollStatus,
    expired,
    stopPoll,
    onPaid,
    onOpenChange,
    t,
  ])

  const handleCopyAmount = () => {
    if (!amountStr) return
    void copyToClipboard(amountStr)
  }

  const handleCopyAddress = () => {
    if (!usdt?.payment_address) return
    void copyToClipboard(usdt.payment_address)
  }

  const handleOpenPaymentUrl = () => {
    if (!usdt?.payment_url) return
    window.open(usdt.payment_url, '_blank', 'noopener,noreferrer')
  }

  const fxRate = usdt?.usdt_fx_rate
  const tierAmount =
    typeof order?.amount_usd === 'number'
      ? order.amount_usd
      : typeof order?.amount === 'number'
        ? order.amount
        : undefined

  const statusLabel = (() => {
    if (pollStatus === 'paid') return t('Paid')
    if (pollStatus === 'need_manual') return t('Needs manual review')
    if (pollStatus === 'failed' || expired) return t('Expired / failed')
    return t('Waiting for transfer')
  })()

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('USDT Payment')}
      description={
        orderNo
          ? t('Order {{no}} · confirmed on-chain automatically', { no: orderNo })
          : t('Transfer the exact USDT amount on TRON TRC20')
      }
      contentClassName='sm:max-w-lg'
      showCloseButton
    >
      {!usdt ? (
        <p className='text-muted-foreground text-sm'>{t('No cashier data')}</p>
      ) : (
        <div className='space-y-4'>
          <div>
            <div className='text-muted-foreground text-[13px]'>
              {t('Amount due (transfer exact amount)')}
            </div>
            <div className='mt-1 flex flex-wrap items-baseline gap-2'>
              <span className='font-mono text-[28px] font-semibold tracking-tight'>
                {amountStr || '—'}
              </span>
              <span className='text-muted-foreground text-sm'>
                {usdt.payment_currency || 'USDT'}
              </span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 gap-1.5 px-2.5 text-xs'
                onClick={handleCopyAmount}
                disabled={!amountStr}
              >
                <Copy className='h-3.5 w-3.5' />
                {t('Copy')}
              </Button>
            </div>
          </div>

          <div>
            <div className='text-muted-foreground mb-1 text-[13px]'>
              {t('Receiving address')}
            </div>
            <div className='bg-muted/40 flex items-center justify-between gap-2 rounded-lg border px-3 py-2'>
              <span className='font-mono text-[12.5px] break-all'>
                {usdt.payment_address}
              </span>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='h-7 shrink-0 gap-1.5 px-2.5 text-xs'
                onClick={handleCopyAddress}
              >
                <Copy className='h-3.5 w-3.5' />
                {t('Copy')}
              </Button>
            </div>
            <p className='text-muted-foreground mt-1.5 text-xs'>
              {t('Network: TRON (TRC20) only. Wrong network may lose funds.')}
            </p>
          </div>

          <div className='grid grid-cols-1 gap-2 text-[13px] sm:grid-cols-2'>
            <InfoRow
              label={t('Top-up tier')}
              value={
                tierAmount != null ? formatUsdAmount(Number(tierAmount)) : '—'
              }
            />
            <InfoRow
              label={t('Locked rate')}
              value={
                typeof fxRate === 'number' && Number.isFinite(fxRate)
                  ? t('1 USD = {{rate}} USDT', {
                      rate: fxRate.toFixed(4),
                    })
                  : '—'
              }
            />
            <InfoRow label={t('Order no.')} value={orderNo || '—'} mono />
            <InfoRow
              label={t('Status')}
              value={
                <span className='inline-flex items-center gap-1.5'>
                  {polling &&
                    pollStatus !== 'paid' &&
                    pollStatus !== 'failed' &&
                    pollStatus !== 'need_manual' && (
                      <Loader2 className='h-3.5 w-3.5 animate-spin' />
                    )}
                  {statusLabel}
                </span>
              }
            />
          </div>

          <div
            className={cn(
              'flex items-center gap-3 rounded-lg border px-4 py-3',
              expired
                ? 'border-destructive/30 bg-destructive/5 text-destructive'
                : 'border-primary/20 bg-primary/5 text-primary'
            )}
          >
            <Clock className='h-5 w-5 shrink-0' />
            <div className='text-[13px]'>
              {expired ? (
                t('Payment window closed. Do not transfer again.')
              ) : (
                <>
                  {t('Time remaining')}{' '}
                  <b className='font-mono text-lg'>{formatCountdown(remaining)}</b>
                  {t(', do not transfer after timeout')}
                </>
              )}
            </div>
          </div>

          {usdt.payment_url ? (
            <Button
              type='button'
              variant='secondary'
              className='w-full gap-2'
              onClick={handleOpenPaymentUrl}
            >
              <ExternalLink className='h-4 w-4' />
              {t('Open payment page')}
            </Button>
          ) : null}

          <Alert className='border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100'>
            <AlertTriangle className='h-4 w-4' />
            <AlertTitle className='text-[13px]'>
              {t('Underpay / overpay')}
            </AlertTitle>
            <AlertDescription className='text-[12.5px] leading-5'>
              {t(
                'If the amount received is less than due, the order will not credit automatically and needs manual review. Contact support with TxID. Overpay also needs manual check.'
              )}
            </AlertDescription>
          </Alert>
        </div>
      )}
    </Dialog>
  )
}

function InfoRow({
  label,
  value,
  mono,
}: {
  label: string
  value: ReactNode
  mono?: boolean
}) {
  return (
    <div className='flex justify-between gap-3 border-b border-dashed border-border/80 pb-2'>
      <span className='text-muted-foreground shrink-0'>{label}</span>
      <span
        className={cn(
          'text-right font-medium break-all',
          mono && 'font-mono text-[12px]'
        )}
      >
        {value}
      </span>
    </div>
  )
}
