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

import { Info, RefreshCw, Search, Undo2 } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatAffUsd } from '@/features/affiliate/lib/quota'
import {
  orderStatusLabelKey,
  paymentLabel,
  refundStatusLabelKey,
} from '@/features/system-settings/orders/lib/orders'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { cn } from '@/lib/utils'

import { ConfirmRefundDialog } from './components/confirm-refund-dialog'
import { useTopupOrderLookup } from './hooks/use-order-refund'
import { canRefundOrder, orderCreditUsd, refundHintUsd } from './lib/refundable'
import type { RefundTopupOrderResult, TopupOrder } from './types'

function refundDisabledHint(
  order: { status: string; refund_status?: string },
  t: (key: string) => string
): string {
  if (order.status !== 'paid') {
    return t('Only paid orders can be refunded.')
  }
  if (order.refund_status === 'full') {
    return t('Order is fully refunded; further refunds are disabled.')
  }
  return t('No refundable amount on this order (hint).')
}

function fmtUsd(n: number): string {
  return formatBillingCurrencyFromUSD(n, {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })
}

function refundBadgeClass(status: string): string {
  if (status === 'full') {
    return 'border-rose-200 bg-rose-50 text-rose-800'
  }
  if (status === 'partial') {
    return 'border-amber-200 bg-amber-50 text-amber-900'
  }
  return 'border-slate-200 bg-slate-50 text-slate-700'
}

function Field({
  label,
  value,
  mono,
}: {
  label: string
  value: ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid gap-0.5'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className={cn('text-sm break-all', mono && 'font-mono')}>
        {value}
      </div>
    </div>
  )
}

function OrderDetailCard({
  order,
  onRefund,
}: {
  order: TopupOrder
  onRefund: () => void
}) {
  const { t } = useTranslation()
  const hint = refundHintUsd(order)
  const allow = canRefundOrder(order)

  return (
    <div className='bg-card rounded-xl border p-4 shadow-sm'>
      <div className='mb-3 flex flex-wrap items-center gap-2'>
        <h3 className='text-sm font-medium'>{t('Order detail')}</h3>
        <Badge variant='outline' className='text-[11px]'>
          {t(orderStatusLabelKey(order.status))}
        </Badge>
        <Badge
          variant='outline'
          className={cn(
            'text-[11px]',
            refundBadgeClass(order.refund_status || 'none')
          )}
        >
          {t('Refund')}:{' '}
          {t(refundStatusLabelKey(order.refund_status || 'none'))}
        </Badge>
        {allow ? (
          <Button
            type='button'
            size='sm'
            className='ml-auto h-8'
            variant='destructive'
            onClick={onRefund}
          >
            <Undo2 className='size-3.5' />
            {t('Refund')}
          </Button>
        ) : null}
      </div>
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
        <Field label={t('Order No.')} value={order.order_no} mono />
        <Field label={t('User')} value={String(order.user_id)} />
        <Field label={t('Payment method')} value={paymentLabel(order, t)} />
        <Field
          label={t('Paid amount')}
          value={fmtUsd(Number(order.paid_amount_usd) || 0)}
        />
        <Field
          label={t('Credit amount')}
          value={fmtUsd(orderCreditUsd(order))}
        />
        <Field
          label={t('Bonus amount')}
          value={fmtUsd(Number(order.bonus_amount_usd) || 0)}
        />
        <Field
          label={t('Already refunded')}
          value={fmtUsd(Number(order.refunded_amount) || 0)}
        />
        <Field
          label={t('Refundable hint')}
          value={
            <>
              {fmtUsd(hint)}{' '}
              <span className='text-muted-foreground text-xs'>
                ({t('system validates final amount')})
              </span>
            </>
          }
        />
      </div>
      {!allow ? (
        <p className='text-muted-foreground mt-3 text-xs'>
          {refundDisabledHint(order, t)}
        </p>
      ) : null}
    </div>
  )
}

function ClawbackSummary({ result }: { result: RefundTopupOrderResult }) {
  const { t } = useTranslation()
  return (
    <div className='rounded-xl border border-emerald-200 bg-emerald-50/70 p-4 text-sm text-emerald-950 dark:border-emerald-900/40 dark:bg-emerald-950/30 dark:text-emerald-100'>
      <div className='mb-2 font-medium'>{t('Clawback summary')}</div>
      <div className='grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-3'>
        <div>
          {t('Paid clawed')}: {formatAffUsd(result.paid_clawed)}
        </div>
        <div>
          {t('Bonus clawed')}: {formatAffUsd(result.bonus_clawed)}
        </div>
        <div>
          {t('Bonus shortfall')}: {formatAffUsd(result.bonus_shortfall)}
        </div>
        <div>
          {t('Affiliate clawed')}: {formatAffUsd(result.aff_clawed)}
        </div>
        <div>
          {t('Refundable before')}:{' '}
          {fmtUsd(Number(result.refundable_before) || 0)}
        </div>
        <div>
          {t('Refund status')}:{' '}
          <span>
            {t(refundStatusLabelKey(result.order?.refund_status || 'none'))}
          </span>
        </div>
        <div>
          {t('Paid before / after')}: {formatAffUsd(result.paid_before)} →{' '}
          {formatAffUsd(result.paid_after)}
        </div>
        <div>
          {t('Bonus before / after')}: {formatAffUsd(result.bonus_before)} →{' '}
          {formatAffUsd(result.bonus_after)}
        </div>
        <div>
          {t('Aff before / after')}: {formatAffUsd(result.aff_before)} →{' '}
          {formatAffUsd(result.aff_after)}
        </div>
      </div>
      {result.bonus_shortfall > 0 ? (
        <p className='mt-2 text-xs'>
          {t(
            'Bonus shortfall means gift balance was insufficient; refund still succeeded.'
          )}
        </p>
      ) : null}
    </div>
  )
}

export function OrderRefundsSettings() {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')
  const [lookupNo, setLookupNo] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [lastResult, setLastResult] = useState<RefundTopupOrderResult | null>(
    null
  )

  const query = useTopupOrderLookup(lookupNo)
  const order = query.data ?? null
  const isForbidden =
    (query.error as { status?: number } | null)?.status === 403
  const isNotFound = (query.error as { status?: number } | null)?.status === 404

  function onSearch(e?: FormEvent) {
    e?.preventDefault()
    const no = draft.trim()
    setLastResult(null)
    setLookupNo(no || null)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Order refunds')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          disabled={!lookupNo}
          onClick={() => void query.refetch()}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Look up a top-up order by order number, then record a partial or full ledger refund. No order list API — search only.'
            )}
          </p>

          <div className='flex gap-2.5 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-[13px] leading-5 text-amber-950 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100'>
            <Info className='mt-0.5 size-4 shrink-0' />
            <div>
              {t(
                'Ledger only — does not call the payment channel for a real refund.'
              )}
            </div>
          </div>

          <form
            className='flex flex-wrap items-center gap-2'
            onSubmit={onSearch}
          >
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t('Enter order number')}
              className='h-9 max-w-md font-mono'
            />
            <Button type='submit' size='sm' className='h-9'>
              <Search className='size-3.5' />
              {t('Search')}
            </Button>
          </form>

          {isForbidden ? (
            <ErrorState
              title={t('You do not have permission to view this order.')}
            />
          ) : null}

          {lookupNo && query.isLoading ? (
            <div className='text-muted-foreground flex min-h-32 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}

          {lookupNo && isNotFound ? (
            <ErrorState
              title={t('Order not found')}
              onRetry={() => void query.refetch()}
            />
          ) : null}

          {lookupNo && query.isError && !isForbidden && !isNotFound ? (
            <ErrorState
              title={t('Failed to load order')}
              onRetry={() => void query.refetch()}
            />
          ) : null}

          {order ? (
            <OrderDetailCard
              order={order}
              onRefund={() => setDialogOpen(true)}
            />
          ) : null}

          {lastResult ? <ClawbackSummary result={lastResult} /> : null}

          <ConfirmRefundDialog
            order={order}
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            onDone={(result) => {
              setLastResult(result)
              void query.refetch()
            }}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
