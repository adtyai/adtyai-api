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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

import { adtyTopupQueryKeys, cancelAdtyOrder, getAdtyOrder } from '../api'
import { payWithHosted } from '../api-pay'
import { formatCountdown, useNowSeconds } from '../lib/countdown'
import { ORDER_STATUS_LABELS } from '../lib/methods'
import { creditedUsdAmount, formatPaymentAmount } from '../lib/money'
import { clearPendingPayment, savePendingPayment } from '../lib/pending-payment'
import { isSafeHostedPaymentUrl } from '../lib/safe-url'

/** Hosted checkout remains recoverable while the gateway runs in another tab. */
export function HostedPayDialog(props: {
  paymentScene?: string
  method: string
  amount: number
  promoCode?: string
  tradeNo?: string
  onClose: () => void
  onSettled: () => Promise<void> | void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [tradeNo, setTradeNo] = useState(props.tradeNo)
  const initiated = useRef(false)
  const settled = useRef(false)
  const setPaymentDialogOpen = useAdtyUiStore(
    (state) => state.setPaymentDialogOpen
  )
  useEffect(() => {
    setPaymentDialogOpen(true)
    return () => setPaymentDialogOpen(false)
  }, [setPaymentDialogOpen])
  const create = useMutation({
    mutationFn: () =>
      payWithHosted(
        props.amount,
        props.method,
        props.promoCode,
        props.paymentScene
      ),
    onSuccess: (result) => {
      setTradeNo(result.tradeNo)
      void queryClient.invalidateQueries({ queryKey: adtyTopupQueryKeys.info })
    },
  })
  const { mutate: createOrder } = create
  useEffect(() => {
    if (!props.tradeNo && !initiated.current) {
      initiated.current = true
      createOrder()
    }
  }, [props.tradeNo, createOrder])
  const query = useQuery({
    queryKey: adtyTopupQueryKeys.order(tradeNo ?? ''),
    queryFn: () => getAdtyOrder(tradeNo ?? ''),
    enabled: Boolean(tradeNo),
    refetchInterval: (current) =>
      current.state.data?.status === 'pending' ? 3000 : false,
  })
  const order = query.data
  const ext = order?.ext
  const status = order?.status
  const now = useNowSeconds(status === 'pending')
  const payUrl = ext?.pay_url || create.data?.payUrl || ''
  const safeUrl = isSafeHostedPaymentUrl(payUrl)
  const { refetch } = query
  useEffect(() => {
    const onVisible = () => {
      if (tradeNo && document.visibilityState === 'visible') void refetch()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [tradeNo, refetch])
  const { onSettled } = props
  useEffect(() => {
    if (!order) return
    if (order.status === 'pending' && order.ext) {
      savePendingPayment(localStorage, {
        tradeNo: order.trade_no,
        method: order.payment_method,
        expireAt: order.ext.expire_at,
      })
    } else if (order.status !== 'pending' && !settled.current) {
      settled.current = true
      clearPendingPayment(localStorage)
      void onSettled()
    }
  }, [order, onSettled])
  const cancel = useMutation({
    mutationFn: () => cancelAdtyOrder(tradeNo ?? ''),
    onSuccess: () => {
      settled.current = true
      clearPendingPayment(localStorage)
      void props.onSettled()
      props.onClose()
    },
  })

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && props.onClose()}
      title={t('Continue payment')}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        status === 'pending' ? (
          <Button
            variant='ghost'
            disabled={cancel.isPending}
            onClick={() => cancel.mutate()}
          >
            {t('Cancel order')}
          </Button>
        ) : (
          <Button onClick={props.onClose}>{t('Close')}</Button>
        )
      }
    >
      {create.isPending && (
        <p className='text-muted-foreground text-sm'>
          {t('Creating the order...')}
        </p>
      )}
      {create.isError && (
        <ErrorState
          description={getServerErrorMessage(create.error)}
          onRetry={() => create.mutate()}
        />
      )}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {cancel.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {getServerErrorMessage(cancel.error)}
        </p>
      )}
      {ext && status === 'success' && (
        <p
          role='status'
          className='text-success text-center text-lg font-semibold'
        >
          {t('Credited ${{amount}}', { amount: creditedUsdAmount(ext) })}
        </p>
      )}
      {status && status !== 'pending' && status !== 'success' && (
        <p role='status'>{t(ORDER_STATUS_LABELS[status])}</p>
      )}
      {status === 'pending' && ext && (
        <div className='flex flex-col items-center gap-3'>
          <p className='text-2xl font-semibold'>
            {formatPaymentAmount(ext.pay_amount, ext.pay_currency, locale)}
          </p>
          <p>{t('You get ${{amount}}', { amount: creditedUsdAmount(ext) })}</p>
          {safeUrl ? (
            <a
              className='text-primary underline'
              href={payUrl}
              target='_blank'
              rel='noopener noreferrer'
            >
              {t('Continue payment')}
            </a>
          ) : (
            <Alert variant='destructive'>
              <AlertDescription>
                {t(
                  'The payment link is not safe to open. Try another channel.'
                )}
              </AlertDescription>
            </Alert>
          )}
          <p>
            {t('Time left')}:{' '}
            <span className='font-mono'>
              {formatCountdown(ext.expire_at, now)}
            </span>
          </p>
        </div>
      )}
    </Dialog>
  )
}
