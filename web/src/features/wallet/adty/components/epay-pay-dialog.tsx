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
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

import { adtyTopupQueryKeys, cancelAdtyOrder, getAdtyOrder } from '../api'
import { payWithEpay } from '../api-pay'
import { formatCountdown, useNowSeconds } from '../lib/countdown'
import { detectPayDevice } from '../lib/device'
import { creditedUsdAmount } from '../lib/money'
import { clearPendingPayment, savePendingPayment } from '../lib/pending-payment'
import { isDisplayablePaymentQr, isSafePaymentUrl } from '../lib/safe-url'
import type { AdtyOrder, AdtyOrderExt } from '../types'

type EpayPayDialogProps = {
  paymentScene?: string
  method: string
  /** Channel picked on the wallet; failures do not switch to another one. */
  channelId?: number
  amount: number
  /** adty task 12.4 */
  promoCode?: string
  tradeNo?: string
  onClose: () => void
  onSettled: () => Promise<void> | void
  /** Injected in tests; defaults to the browser. */
  device?: 'pc' | 'mobile'
  navigate?: (url: string) => void
}

/** QR content for desktop: the gateway QR code, otherwise the payment link. */
function qrContent(ext: AdtyOrderExt): string {
  if (isDisplayablePaymentQr(ext.qr_code)) return ext.qr_code
  if (isSafePaymentUrl(ext.pay_url)) return ext.pay_url
  return ''
}

/** Link for phones, tried in the order qrcode → payurl → urlscheme. */
function mobileTarget(ext: AdtyOrderExt): string {
  for (const candidate of [ext.qr_code, ext.pay_url, ext.url_scheme]) {
    if (candidate && isSafePaymentUrl(candidate)) return candidate
  }
  return ''
}

/** Alipay / WeChat payment: QR code on desktop, jump to the app on phones. */
export function EpayPayDialog(props: EpayPayDialogProps) {
  const { t } = useTranslation()
  const device = props.device ?? detectPayDevice()
  const [tradeNo, setTradeNo] = useState<string | undefined>(props.tradeNo)
  const [blocked, setBlocked] = useState(false)
  const setPaymentDialogOpen = useAdtyUiStore(
    (state) => state.setPaymentDialogOpen
  )
  const settledRef = useRef(false)
  const jumpedRef = useRef<string>('')

  useEffect(() => {
    setPaymentDialogOpen(true)
    return () => setPaymentDialogOpen(false)
  }, [setPaymentDialogOpen])

  const create = useMutation({
    mutationFn: () =>
      payWithEpay(
        props.amount,
        props.method,
        props.channelId,
        device,
        props.promoCode ?? '',
        props.paymentScene
      ),
    onSuccess: (order) => {
      setTradeNo(order.trade_no)
      if (order.ext) {
        savePendingPayment(window.localStorage, {
          tradeNo: order.trade_no,
          method: order.payment_method,
          expireAt: order.ext.expire_at,
        })
      }
    },
    onError: (error) => {
      handleServerError(error)
      props.onClose()
    },
  })
  const { mutate: createOrder } = create
  useEffect(() => {
    if (!props.tradeNo) createOrder()
  }, [props.tradeNo, createOrder])

  const orderQuery = useQuery({
    queryKey: adtyTopupQueryKeys.order(tradeNo ?? ''),
    queryFn: () => getAdtyOrder(tradeNo ?? ''),
    enabled: Boolean(tradeNo),
    refetchInterval: (query) =>
      query.state.data?.status === 'pending' ? 3000 : false,
  })
  const order: AdtyOrder | undefined =
    orderQuery.data?.trade_no === tradeNo ? orderQuery.data : create.data
  const status = order?.status
  const now = useNowSeconds(status === 'pending')
  const ext = order?.ext

  const { refetch } = orderQuery
  useEffect(() => {
    // Back from the payment app: check the order right away.
    const onVisible = () => {
      if (document.visibilityState === 'visible') void refetch()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [refetch])

  const navigate = props.navigate
  useEffect(() => {
    if (
      device !== 'mobile' ||
      !ext ||
      status !== 'pending' ||
      jumpedRef.current === ext.trade_no
    ) {
      return
    }
    jumpedRef.current = ext.trade_no
    const target = mobileTarget(ext)
    if (!target) {
      setBlocked(true)
      return
    }
    if (navigate) {
      navigate(target)
      return
    }
    window.location.assign(target)
  }, [device, ext, status, navigate])

  const { onSettled } = props
  useEffect(() => {
    if (status && status !== 'pending' && !settledRef.current) {
      settledRef.current = true
      clearPendingPayment(window.localStorage)
      void onSettled()
    }
  }, [status, onSettled])

  const cancel = useMutation({
    mutationFn: (no: string) => cancelAdtyOrder(no),
  })

  const cancelOrder = async () => {
    if (tradeNo) await cancel.mutateAsync(tradeNo)
    clearPendingPayment(window.localStorage)
    props.onClose()
  }

  const appName = props.method === 'wxpay' ? t('WeChat') : t('Alipay')
  const qr = ext ? qrContent(ext) : ''
  const unsafeLink =
    blocked ||
    (device === 'pc' &&
      status === 'pending' &&
      qr === '' &&
      Boolean(ext?.qr_code || ext?.pay_url || ext?.url_scheme))

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={
        props.method === 'wxpay'
          ? t('Pay with WeChat Pay')
          : t('Pay with Alipay')
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        status === 'pending' ? (
          <>
            <Button
              variant='outline'
              disabled={cancel.isPending || create.isPending}
              onClick={() => void cancelOrder()}
            >
              {t('Choose another payment method')}
            </Button>
            <Button
              variant='ghost'
              disabled={cancel.isPending}
              onClick={() => void cancelOrder()}
            >
              {t('Cancel order')}
            </Button>
          </>
        ) : (
          <Button onClick={props.onClose}>{t('Close')}</Button>
        )
      }
    >
      {!ext && (
        <p className='text-muted-foreground text-sm'>
          {t('Creating the order...')}
        </p>
      )}
      {ext && status === 'success' && (
        <p
          className='text-success text-center text-lg font-semibold'
          role='status'
        >
          {t('Credited ${{amount}}', {
            amount: creditedUsdAmount(ext),
          })}
        </p>
      )}
      {ext && status === 'expired' && (
        <p className='text-destructive text-center' role='status'>
          {t('This order has expired. Please create a new one.')}
        </p>
      )}
      {unsafeLink && (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'The payment link is not safe to open. Choose another payment method.'
            )}
          </AlertDescription>
        </Alert>
      )}
      {ext && status === 'pending' && (
        <div className='flex flex-col items-center gap-3'>
          <p className='text-2xl font-semibold' data-testid='epay-amount'>
            ¥{Number(ext.pay_amount).toFixed(2)}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t('You get ${{amount}}', {
              amount: creditedUsdAmount(ext),
            })}
          </p>
          {device === 'pc' && qr && (
            <>
              <div
                className='rounded-lg bg-white p-3'
                data-testid='epay-qr'
                data-value={qr}
              >
                <QRCodeSVG value={qr} size={200} />
              </div>
              <p className='text-sm'>
                {t('Scan with {{app}}', { app: appName })}
              </p>
            </>
          )}
          {device === 'mobile' && !blocked && (
            <p className='text-sm'>
              {t('Opening {{app}}... Come back here after paying.', {
                app: appName,
              })}
            </p>
          )}
          <p className='text-sm'>
            {t('Time left')}:{' '}
            <span className='font-mono font-semibold'>
              {formatCountdown(ext.expire_at, now)}
            </span>
          </p>
        </div>
      )}
    </Dialog>
  )
}
