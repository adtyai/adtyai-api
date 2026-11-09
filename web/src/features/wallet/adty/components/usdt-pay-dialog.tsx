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

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

import { adtyTopupQueryKeys, cancelAdtyOrder, getAdtyOrder } from '../api'
import { payWithBEpusdt } from '../api-pay'
import { formatCountdown, useNowSeconds } from '../lib/countdown'
import { CHAIN_NAMES } from '../lib/methods'
import { creditedUsdAmount } from '../lib/money'
import { clearPendingPayment, savePendingPayment } from '../lib/pending-payment'
import type { AdtyOrder } from '../types'
import { ChainIcon } from './chain-icons'

type UsdtPayDialogProps = {
  paymentScene?: string
  chain: string
  amount: number
  /** adty task 12.4 */
  promoCode?: string
  /** Resume this order instead of creating a new one. */
  tradeNo?: string
  onClose: () => void
  onSettled: () => Promise<void> | void
  /** Goes back to the recharge card so the user can pick another chain. */
  onChangeChain: () => void
}

/** In-site USDT payment: address QR code, amount, countdown and polling. */
export function UsdtPayDialog(props: UsdtPayDialogProps) {
  const { t } = useTranslation()
  const [tradeNo, setTradeNo] = useState<string | undefined>(props.tradeNo)
  const [notice, setNotice] = useState('')
  const setPaymentDialogOpen = useAdtyUiStore(
    (state) => state.setPaymentDialogOpen
  )
  const settledRef = useRef(false)

  useEffect(() => {
    setPaymentDialogOpen(true)
    return () => setPaymentDialogOpen(false)
  }, [setPaymentDialogOpen])

  const create = useMutation({
    mutationFn: () =>
      payWithBEpusdt(
        props.amount,
        props.chain,
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
  const order: AdtyOrder | undefined = orderQuery.data ?? create.data
  const status = order?.status
  const now = useNowSeconds(status === 'pending')

  const { onSettled } = props
  useEffect(() => {
    if (status && status !== 'pending' && !settledRef.current) {
      settledRef.current = true
      clearPendingPayment(window.localStorage)
      void onSettled()
    }
  }, [status, onSettled])

  const cancel = useMutation({
    mutationFn: () => cancelAdtyOrder(tradeNo ?? ''),
  })

  const handleCancel = async (thenChangeChain: boolean) => {
    try {
      await cancel.mutateAsync()
      clearPendingPayment(window.localStorage)
      if (thenChangeChain) {
        setNotice(t('Do not send anything to the old address.'))
        props.onChangeChain()
        return
      }
      props.onClose()
    } catch {
      // The gateway refuses to cancel once it has seen the transfer.
      setNotice(t('A transfer has been detected. Please wait'))
    }
  }

  const ext = order?.ext
  const chainName = CHAIN_NAMES[props.chain] ?? props.chain
  const expired =
    status === 'expired' ||
    (ext ? ext.expire_at <= now && status === 'pending' : false)

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Pay with USDT')}
      description={t('Only send USDT on the {{chain}} network.', {
        chain: chainName,
      })}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        status === 'pending' ? (
          <>
            <Button
              variant='outline'
              disabled={cancel.isPending}
              onClick={() => void handleCancel(true)}
            >
              {t('Change network')}
            </Button>
            <Button
              variant='ghost'
              disabled={cancel.isPending}
              onClick={() => void handleCancel(false)}
            >
              {t('Cancel order')}
            </Button>
          </>
        ) : (
          <Button onClick={props.onClose}>{t('Close')}</Button>
        )
      }
    >
      {notice && (
        <Alert>
          <AlertDescription>{notice}</AlertDescription>
        </Alert>
      )}
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
      {ext && expired && status !== 'success' && (
        <p className='text-destructive text-center' role='status'>
          {t('This order has expired. Please create a new one.')}
        </p>
      )}
      {ext && status === 'pending' && !expired && (
        <div className='flex flex-col items-center gap-4'>
          <div className='flex items-center gap-2 text-sm font-medium'>
            <ChainIcon method={props.chain} />
            <span>USDT · {chainName}</span>
          </div>
          <div className='rounded-lg bg-white p-3'>
            <QRCodeSVG value={ext.pay_address} size={180} />
          </div>
          <div className='w-full space-y-1'>
            <p className='text-muted-foreground text-xs'>
              {t('Receiving address')}
            </p>
            <div className='bg-muted flex items-center justify-between gap-2 rounded-md p-2'>
              <code
                className='font-mono text-xs break-all'
                data-testid='usdt-address'
              >
                {ext.pay_address}
              </code>
              <CopyButton
                value={ext.pay_address}
                aria-label={t('Copy address')}
              />
            </div>
          </div>
          <div className='w-full space-y-1'>
            <p className='text-muted-foreground text-xs'>
              {t('Amount to send')}
            </p>
            <div className='bg-muted flex items-center justify-between gap-2 rounded-md p-2'>
              <code
                className='font-mono text-base font-semibold'
                data-testid='usdt-amount'
              >
                {ext.pay_amount} USDT
              </code>
              <CopyButton
                value={ext.pay_amount}
                aria-label={t('Copy amount')}
              />
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Send exactly the amount shown; it may include an extra 0.01.'
              )}
            </p>
          </div>
          <p className='text-sm'>
            {t('Time left')}:{' '}
            <span
              className='font-mono font-semibold'
              data-testid='usdt-countdown'
            >
              {formatCountdown(ext.expire_at, now)}
            </span>
          </p>
          <Alert variant='destructive'>
            <AlertDescription>
              {t(
                'Only the {{chain}} network is accepted. Transfers on other networks cannot be recovered.',
                {
                  chain: chainName,
                }
              )}
            </AlertDescription>
          </Alert>
        </div>
      )}
    </Dialog>
  )
}
