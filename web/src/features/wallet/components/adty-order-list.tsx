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
import { Loader2, Receipt } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { TitledCard } from '@/components/ui/titled-card'
import { usdtChainById } from '@/lib/usdt-chains'

import { continueAdtyTopupOrder, listAdtyTopupOrders } from '../api'
import {
  formatUsdAmount,
  paymentMethodLabelKey,
  resolveAdtyCheckoutLaunch,
  topupNextActionText,
} from '../lib/adty-topup'
import { submitPaymentForm } from '../lib/payment'
import type {
  AdtySelfTopupOrder,
  AdtyTopupOrder,
  AdtyUsdtCashier,
} from '../types'
import { AdtyUsdtCashierDialog } from './adty-usdt-cashier-dialog'

const PAGE_SIZE = 10

function statusLabel(status: string, t: (key: string) => string): string {
  switch (status) {
    case 'paid':
      return t('Credited')
    case 'review':
      return t('Under review')
    case 'expired':
      return t('Expired')
    case 'failed':
      return t('Payment failed')
    default:
      return t('Pending payment')
  }
}

function formatOrderTime(unixSeconds: number): string {
  if (!unixSeconds) return '—'
  return new Date(unixSeconds * 1000).toLocaleString()
}

function refundLabelKey(status: string | undefined): string {
  switch ((status || '').trim()) {
    case 'partial':
      return 'Refund partial'
    case 'full':
      return 'Refund full'
    default:
      return ''
  }
}

function finiteUsd(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

export function AdtyOrderList({
  onPaid,
  refreshToken = 0,
}: {
  onPaid?: () => void
  refreshToken?: number
}) {
  const { t } = useTranslation()
  const [items, setItems] = useState<AdtySelfTopupOrder[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [continuing, setContinuing] = useState('')
  const [usdtOpen, setUsdtOpen] = useState(false)
  const [usdt, setUsdt] = useState<AdtyUsdtCashier | null>(null)
  const [order, setOrder] = useState<AdtyTopupOrder | null>(null)

  const load = useCallback(async (nextPage: number, append: boolean) => {
    setLoading(true)
    try {
      const res = await listAdtyTopupOrders(nextPage, PAGE_SIZE)
      if (!res?.success || !res.data) {
        if (!append) {
          setItems([])
          setTotal(0)
        }
        return
      }
      const data = res.data
      setPage(data.page || nextPage)
      setTotal(data.total || 0)
      setItems((prev) =>
        append ? [...prev, ...(data.items || [])] : data.items || []
      )
    } catch {
      if (!append) {
        setItems([])
        setTotal(0)
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load(1, false)
  }, [load, refreshToken])

  const continuePay = async (row: AdtySelfTopupOrder) => {
    setContinuing(row.order_no)
    try {
      const res = await continueAdtyTopupOrder(row.order_no)
      if (!res?.success) {
        if (res?.code === 'order_expired') {
          toast.error(t('This order has expired. Please create a new top-up.'))
        } else if (res?.code === 'order_not_payable') {
          toast.error(t('This order cannot be paid now'))
        } else {
          toast.error(res?.message || t('Cannot continue this payment'))
        }
        await load(1, false)
        return
      }
      const launch = resolveAdtyCheckoutLaunch(res.data)
      if (launch.kind === 'usdt') {
        setOrder(launch.order)
        setUsdt(launch.usdt)
        setUsdtOpen(true)
        return
      }
      if (launch.kind === 'epay') {
        if (launch.payParams && Object.keys(launch.payParams).length > 0) {
          submitPaymentForm(launch.payUrl, launch.payParams)
        } else {
          window.open(launch.payUrl, '_blank', 'noopener,noreferrer')
        }
        toast.success(t('Redirecting to payment page...'))
        return
      }
      toast.error(t('Cannot continue this payment'))
    } catch {
      toast.error(t('Cannot continue this payment'))
    } finally {
      setContinuing('')
    }
  }

  return (
    <>
      <TitledCard
        title={t('My top-up orders')}
        icon={<Receipt className='h-4 w-4' />}
        disableHoverEffect
        contentClassName='space-y-2'
      >
        {items.length === 0 && !loading ? (
          <p className='text-muted-foreground text-sm'>
            {t('No top-up orders yet')}
          </p>
        ) : (
          <ul className='divide-y'>
            {items.map((row) => {
              const chain =
                row.usdt_network || usdtChainById(row.usdt_chain)?.network || ''
              const methodKey = paymentMethodLabelKey(row.payment_method)
              const refundKey = refundLabelKey(row.refund_status)
              const credit = finiteUsd(row.credit_amount_usd)
              const bonus = finiteUsd(row.bonus_amount_usd)
              const refunded = finiteUsd(row.refunded_amount)
              const money = [
                credit == null
                  ? ''
                  : `${t('Credit amount')} ${formatUsdAmount(credit)}`,
                bonus == null
                  ? ''
                  : `${t('Bonus amount')} ${formatUsdAmount(bonus)}`,
                refundKey && refunded != null
                  ? `${t('Already refunded')} ${formatUsdAmount(refunded)}`
                  : '',
              ]
                .filter(Boolean)
                .join(' · ')
              return (
                <li
                  key={row.order_no}
                  className='flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between'
                >
                  <div className='min-w-0'>
                    <div className='text-sm font-medium'>
                      {formatUsdAmount(row.amount_usd)}
                      <span className='text-muted-foreground ml-2 text-xs font-normal'>
                        {statusLabel(row.display_status, t)}
                        {refundKey ? ` · ${t(refundKey)}` : ''}
                      </span>
                    </div>
                    {money ? (
                      <div className='text-muted-foreground text-xs'>{money}</div>
                    ) : null}
                    {topupNextActionText(row) ? (
                      <div className='text-muted-foreground text-xs'>
                        {t(topupNextActionText(row))}
                      </div>
                    ) : null}
                    <div className='text-muted-foreground truncate text-xs'>
                      {methodKey ? `${t(methodKey)} · ` : ''}
                      {row.order_no}
                      {chain ? ` · ${chain}` : ''}
                      {` · ${formatOrderTime(row.create_time)}`}
                    </div>
                  </div>
                  {row.can_continue ? (
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={continuing === row.order_no}
                      onClick={() => void continuePay(row)}
                    >
                      {continuing === row.order_no && (
                        <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                      )}
                      {t('Continue payment')}
                    </Button>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
        {items.length < total ? (
          <Button
            variant='ghost'
            size='sm'
            disabled={loading}
            onClick={() => void load(page + 1, true)}
          >
            {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Load more')}
          </Button>
        ) : null}
      </TitledCard>
      <AdtyUsdtCashierDialog
        open={usdtOpen}
        onOpenChange={setUsdtOpen}
        order={order}
        usdt={usdt}
        onPaid={() => {
          void load(1, false)
          onPaid?.()
        }}
        onTerminal={() => {
          void load(1, false)
        }}
      />
    </>
  )
}
