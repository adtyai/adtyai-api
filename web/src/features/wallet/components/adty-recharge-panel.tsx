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
import { Link } from '@tanstack/react-router'
import {
  Check,
  Loader2,
  Receipt,
  WalletCards,
  AlertTriangle,
} from 'lucide-react'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { usdtChainById } from '@/lib/usdt-chains'
import { cn } from '@/lib/utils'

import { createAdtyTopupOrder } from '../api'
import {
  computeAdtyBonusPlan,
  computePayLocalAmount,
  findTier,
  firstTopupBonusRateToPercent,
  formatUsdAmount,
  isFixedTierAmount,
  resolveAdtyCheckoutLaunch,
} from '../lib/adty-topup'
import { formatCurrency } from '../lib/format'
import { submitPaymentForm } from '../lib/payment'
import type {
  AdtyPayMethod,
  AdtyTopupOrder,
  AdtyTopupTier,
  AdtyUsdtCashier,
  AdtyUsdtChain,
} from '../types'
import { AdtyUsdtCashierDialog } from './adty-usdt-cashier-dialog'

export interface AdtyRechargePanelProps {
  tiers: AdtyTopupTier[]
  payMethods: AdtyPayMethod[]
  usdtChains?: AdtyUsdtChain[]
  firstTopupBonusRate: number
  firstEligible: boolean
  loading?: boolean
  /** status.price — local currency per 1 USD credit */
  price?: number
  redemptionCode?: string
  onRedemptionCodeChange?: (code: string) => void
  onRedeem?: () => void
  redeeming?: boolean
  redemptionEnabled?: boolean
  /** Refresh balance / first-topup after USDT paid */
  onUsdtPaid?: () => void
  /** Refresh the order list when a payment needs review or fails. */
  onUsdtTerminal?: () => void
}

function payMethodLabel(method: AdtyPayMethod, t: (k: string) => string) {
  if (method.type === 'alipay') return t('Alipay')
  if (method.type === 'wxpay' || method.type === 'wechat') {
    return t('WeChat Pay')
  }
  if (method.type === 'usdt') return t('USDT')
  return method.name || method.type
}

function payMethodHint(method: AdtyPayMethod, t: (k: string) => string) {
  if (!method.available) {
    return method.maintenance_reason || t('Temporarily unavailable')
  }
  if (method.type === 'usdt') return t('On-chain transfer · about 1–3 min')
  return t('CNY payment · instant credit')
}

export function AdtyRechargePanel({
  tiers,
  payMethods,
  usdtChains = [],
  firstTopupBonusRate,
  loading,
  price = 1,
  onUsdtPaid,
  onUsdtTerminal,
}: AdtyRechargePanelProps) {
  const { t } = useTranslation()

  const [selectedAmount, setSelectedAmount] = useState<number | null>(null)
  const [selectedMethod, setSelectedMethod] = useState<AdtyPayMethod | null>(
    null
  )
  const [selectedChain, setSelectedChain] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [usdtOpen, setUsdtOpen] = useState(false)
  const [usdtCashier, setUsdtCashier] = useState<AdtyUsdtCashier | null>(null)
  const [usdtOrder, setUsdtOrder] = useState<AdtyTopupOrder | null>(null)

  useEffect(() => {
    if (tiers.length === 0) return
    setSelectedAmount((prev) => {
      if (prev != null && isFixedTierAmount(prev, tiers)) return prev
      // Prefer $100 if present, else middle, else first
      const prefer = tiers.find((x) => Number(x.amount) === 100)
      return Number((prefer ?? tiers[Math.min(6, tiers.length - 1)]).amount)
    })
  }, [tiers])

  useEffect(() => {
    if (payMethods.length === 0) {
      setSelectedMethod(null)
      return
    }
    setSelectedMethod((prev) => {
      if (!prev) return null
      return payMethods.find((m) => m.type === prev.type) ?? null
    })
  }, [payMethods])

  useEffect(() => {
    setSelectedChain((prev) => {
      if (prev && usdtChains.some((chain) => chain.id === prev)) return prev
      if (usdtChains.length === 1) return usdtChains[0].id
      return ''
    })
  }, [usdtChains])

  const selectedTier = useMemo(
    () =>
      selectedAmount != null ? findTier(tiers, selectedAmount) : undefined,
    [tiers, selectedAmount]
  )

  const bonusPlan = useMemo(
    () =>
      selectedAmount != null
        ? computeAdtyBonusPlan(
            selectedAmount,
            selectedTier,
            firstTopupBonusRate,
            // UI: always show tier bonus only (first-topup gift removed from display)
            false
          )
        : null,
    [selectedAmount, selectedTier, firstTopupBonusRate]
  )

  const payLocal = useMemo(
    () =>
      selectedAmount != null ? computePayLocalAmount(selectedAmount, price) : 0,
    [selectedAmount, price]
  )

  const unavailableMethods = payMethods.filter((m) => !m.available)
  const anyAvailable = payMethods.some((m) => m.available)
  const methodBlocked = !selectedMethod || !selectedMethod.available
  const needsNetwork =
    selectedMethod?.type === 'usdt' && usdtChains.length > 1 && !selectedChain
  let payButtonLabel = t('Pay now ¥{{amount}}', {
    amount: formatCurrency(payLocal),
  })
  if (!selectedMethod) {
    payButtonLabel = t('Select a payment method')
  } else if (!selectedMethod.available || !anyAvailable) {
    payButtonLabel = t('Payment channel unavailable')
  } else if (needsNetwork) {
    payButtonLabel = t('Choose a USDT network before paying.')
  } else if (selectedMethod.type === 'usdt') {
    payButtonLabel = t('Pay with USDT')
  }

  const handleSelectMethod = (method: AdtyPayMethod) => {
    if (!method.available) {
      toast.error(
        method.maintenance_reason ||
          t('This payment method is under maintenance')
      )
      return
    }
    setSelectedMethod(method)
  }

  const handleSubmit = async () => {
    if (selectedAmount == null || !selectedTier) {
      toast.error(t('Please select a top-up tier'))
      return
    }
    if (!isFixedTierAmount(selectedAmount, tiers)) {
      toast.error(t('Invalid top-up tier. Please choose a listed amount.'))
      return
    }
    if (!selectedMethod) {
      toast.error(t('Please select a payment method'))
      return
    }
    if (!selectedMethod.available) {
      toast.error(
        selectedMethod.maintenance_reason ||
          t('This payment method is under maintenance')
      )
      return
    }
    if (selectedMethod.type === 'usdt' && !selectedChain) {
      toast.error(t('Please select a USDT network'))
      return
    }

    setSubmitting(true)
    try {
      const res = await createAdtyTopupOrder({
        amount: selectedAmount,
        payment_method: selectedMethod.type,
        ...(selectedMethod.type === 'usdt'
          ? { usdt_chain: selectedChain }
          : {}),
      })
      if (!res?.success) {
        const code = res?.code
        if (code === 'payment_maintenance') {
          toast.error(
            res?.message ||
              t('Payment channel is under maintenance. Please try again later.')
          )
          return
        }
        if (code === 'invalid_usdt_chain' || code === 'usdt_chain_disabled') {
          toast.error(res?.message || t('Please select a USDT network'))
          return
        }
        const msg =
          code === 'invalid_tier_amount'
            ? t('Invalid top-up tier. Please choose a listed amount.')
            : res?.message || t('Failed to create top-up order')
        toast.error(msg)
        return
      }

      const launch = resolveAdtyCheckoutLaunch(res.data)
      if (launch.kind === 'usdt') {
        setUsdtOrder(launch.order)
        setUsdtCashier(launch.usdt)
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

      toast.error(t('Failed to open payment checkout'))
    } catch (err) {
      const status = (
        err as {
          response?: {
            status?: number
            data?: { message?: string; code?: string }
          }
        }
      )?.response
      const code = status?.data?.code
      if (status?.status === 503 || code === 'payment_maintenance') {
        toast.error(
          status?.data?.message ||
            t('Payment channel is under maintenance. Please try again later.')
        )
      } else if (status?.status === 400 || code === 'invalid_tier_amount') {
        toast.error(t('Invalid top-up tier. Please choose a listed amount.'))
      } else {
        toast.error(status?.data?.message || t('Failed to create top-up order'))
      }
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <TitledCard
        title={t('Wallet Top-up')}
        description={t('USD pricing · full amount credited · tier bonus extra')}
        icon={<WalletCards className='h-4 w-4' />}
        iconTone='success'
        disableHoverEffect
        contentClassName='space-y-4'
      >
        <Skeleton className='h-20 w-full' />
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4 md:grid-cols-5'>
          {Array.from({ length: 10 }, (_, i) => (
            <Skeleton key={i} className='h-24 rounded-xl' />
          ))}
        </div>
        <Skeleton className='h-16 w-full' />
      </TitledCard>
    )
  }

  return (
    <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(300px,340px)] xl:items-start'>
      <div className='space-y-4'>
        <TitledCard
          title={t('Select top-up tier')}
          description={t('Tier bonus about 1%–8%, increases with amount')}
          icon={<WalletCards className='h-4 w-4' />}
          iconTone='success'
          disableHoverEffect
          action={
            <Button
              variant='outline'
              size='sm'
              className='w-full gap-2 sm:w-auto'
              render={<Link to='/wallet/ledger' />}
            >
              <Receipt className='h-4 w-4' />
              {t('Billing & ledger')}
            </Button>
          }
          contentClassName='space-y-5'
        >
          <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5'>
            {tiers.map((tier) => {
              const amount = Number(tier.amount)
              const selected = selectedAmount === amount
              const tierPct =
                typeof tier.bonus_percent === 'number'
                  ? tier.bonus_percent
                  : firstTopupBonusRateToPercent(tier.bonus_rate)
              const tierLabel =
                tierPct > 0
                  ? t('Top-up bonus {{percent}}%', { percent: tierPct })
                  : null
              return (
                <button
                  key={amount}
                  type='button'
                  onClick={() => setSelectedAmount(amount)}
                  className={cn(
                    'relative rounded-xl border bg-background px-3.5 py-3 text-left transition-colors',
                    selected
                      ? 'border-primary ring-primary/20 bg-primary/5 ring-2'
                      : 'border-border hover:border-primary/40'
                  )}
                >
                  {selected && (
                    <span className='bg-primary text-primary-foreground absolute top-2 right-2 grid h-5 w-5 place-items-center rounded-full'>
                      <Check className='h-3 w-3' />
                    </span>
                  )}
                  <div className='pr-6 text-lg font-semibold tracking-tight tabular-nums'>
                    {formatUsdAmount(amount)}
                  </div>
                  {tierLabel && (
                    <div className='mt-1.5 truncate'>
                      <span className='inline-block max-w-full truncate rounded bg-amber-50 px-1.5 py-0.5 text-[11px] leading-4 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'>
                        {tierLabel}
                      </span>
                    </div>
                  )}
                </button>
              )
            })}
          </div>

          <div className='space-y-2.5'>
            <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Payment Method')}
            </Label>
            <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
              {payMethods.map((method) => {
                const selected = selectedMethod?.type === method.type
                const disabled = !method.available
                return (
                  <button
                    key={method.type}
                    type='button'
                    onClick={() => handleSelectMethod(method)}
                    disabled={disabled && !selected}
                    className={cn(
                      'flex items-center gap-3 rounded-xl border p-3 text-left transition-colors',
                      disabled && 'bg-muted/40 cursor-not-allowed opacity-60',
                      !disabled && selected
                        ? 'border-primary ring-primary/20 ring-2'
                        : !disabled && 'border-border hover:border-primary/40'
                    )}
                    aria-disabled={disabled}
                    title={
                      disabled
                        ? method.maintenance_reason ||
                          t('Temporarily unavailable')
                        : undefined
                    }
                  >
                    <PayMethodBadge type={method.type} disabled={disabled} />
                    <div className='min-w-0'>
                      <div
                        className={cn(
                          'truncate text-sm font-medium',
                          disabled && 'text-muted-foreground'
                        )}
                      >
                        {payMethodLabel(method, t)}
                        {disabled ? ` · ${t('Under maintenance')}` : ''}
                      </div>
                      <div className='text-muted-foreground truncate text-[11.5px]'>
                        {payMethodHint(method, t)}
                      </div>
                    </div>
                  </button>
                )
              })}
            </div>
            {selectedMethod?.type === 'usdt' && usdtChains.length > 0 && (
              <div className='space-y-2'>
                <Label className='text-sm'>{t('USDT network')}</Label>
                <div className='grid grid-cols-2 gap-2'>
                  {usdtChains.map((chain) => {
                    const active = selectedChain === chain.id
                    const known = usdtChainById(chain.id)
                    return (
                      <button
                        key={chain.id}
                        type='button'
                        onClick={() => setSelectedChain(chain.id)}
                        className={cn(
                          'rounded-lg border px-3 py-2 text-left',
                          active
                            ? 'border-primary bg-primary/5'
                            : 'hover:bg-muted/50'
                        )}
                      >
                        <div className='text-sm font-medium'>
                          {known?.label || chain.label || chain.id}
                        </div>
                        <div className='text-muted-foreground text-[11.5px]'>
                          {chain.network || known?.network}
                        </div>
                      </button>
                    )
                  })}
                </div>
              </div>
            )}
            {unavailableMethods.length > 0 && (
              <Alert className='border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100'>
                <AlertTriangle className='h-4 w-4' />
                <AlertDescription className='text-[12.5px]'>
                  {unavailableMethods
                    .map(
                      (m) =>
                        m.maintenance_reason ||
                        `${payMethodLabel(m, t)}: ${t('Temporarily unavailable')}`
                    )
                    .filter((v, i, arr) => arr.indexOf(v) === i)
                    .join(' · ')}
                  {!anyAvailable
                    ? ` ${t('You can wait for the channel or contact support.')}`
                    : ''}
                </AlertDescription>
              </Alert>
            )}
            <p className='text-muted-foreground text-xs'>
              {t(
                'Alipay / WeChat go through partner checkout; bank cards are not supported yet.'
              )}
            </p>
          </div>
        </TitledCard>
      </div>

      <div className='space-y-4 xl:sticky xl:top-4'>
        <TitledCard
          title={t('Order confirmation')}
          icon={<Receipt className='h-4 w-4' />}
          iconTone='info'
          disableHoverEffect
          contentClassName='space-y-3'
        >
          <SummaryRow
            label={t('Top-up tier')}
            value={
              selectedAmount != null ? formatUsdAmount(selectedAmount) : '—'
            }
          />
          <SummaryRow
            label={t('Amount paid (credited in full)')}
            value={
              selectedAmount != null ? formatUsdAmount(selectedAmount) : '—'
            }
          />
          <SummaryRow
            label={t('Top-up extra bonus')}
            value={
              bonusPlan ? (
                <span className='text-amber-600'>
                  {t('{{amount}} credit', {
                    amount: formatUsdAmount(bonusPlan.chosenBonusUSD),
                  })}
                </span>
              ) : (
                '—'
              )
            }
          />
          <SummaryRow
            label={t('Total credited')}
            value={
              bonusPlan ? (
                <span className='text-primary text-base font-semibold'>
                  {formatUsdAmount(bonusPlan.creditTotalUSD)}
                </span>
              ) : (
                '—'
              )
            }
          />
          <SummaryRow
            label={t('Payment Method')}
            value={
              selectedMethod
                ? payMethodLabel(selectedMethod, t)
                : t('Select a payment method')
            }
          />
          {selectedMethod?.type === 'usdt' ? (
            <SummaryRow
              label={t('USDT network')}
              value={
                usdtChains.find((chain) => chain.id === selectedChain)?.label ||
                selectedChain ||
                t('Choose a USDT network before paying.')
              }
            />
          ) : null}
          {selectedMethod && selectedMethod.type !== 'usdt' ? (
            <SummaryRow
              label={t('Pay amount (rate {{rate}})', {
                rate: Number.isFinite(price)
                  ? Number(price).toFixed(2)
                  : '1.00',
              })}
              value={`¥${formatCurrency(payLocal)}`}
            />
          ) : null}
          {selectedMethod?.type === 'usdt' ? (
            <p className='text-muted-foreground text-[11.5px] leading-5'>
              {t(
                'Exact transfer amount is shown on the cashier after the order is created.'
              )}
            </p>
          ) : null}

          <Button
            className='mt-2 h-11 w-full text-[15px]'
            disabled={
              submitting ||
              selectedAmount == null ||
              methodBlocked ||
              !anyAvailable ||
              needsNetwork
            }
            onClick={() => void handleSubmit()}
          >
            {submitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {payButtonLabel}
          </Button>
          <p className='text-muted-foreground text-[11.5px] leading-5'>
            {t(
              'Bonus can be used for API calls, cannot be withdrawn, and does not earn affiliate commission. Spend bonus balance before paid balance.'
            )}
          </p>
        </TitledCard>
      </div>

      <AdtyUsdtCashierDialog
        open={usdtOpen}
        onOpenChange={(next) => {
          setUsdtOpen(next)
          if (!next) {
            setUsdtCashier(null)
            setUsdtOrder(null)
          }
        }}
        order={usdtOrder}
        usdt={usdtCashier}
        onPaid={onUsdtPaid}
        onTerminal={() => onUsdtTerminal?.()}
      />
    </div>
  )
}

function SummaryRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className='border-border/80 flex justify-between gap-3 border-b border-dashed pb-2 text-[13px] last:border-0'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='text-right font-medium'>{value}</span>
    </div>
  )
}

function PayMethodBadge({
  type,
  disabled,
}: {
  type: string
  disabled?: boolean
}) {
  const base =
    'grid h-9 w-9 shrink-0 place-items-center rounded-lg text-sm font-bold text-white'
  if (disabled) {
    return <span className={cn(base, 'bg-slate-400')}>{badgeChar(type)}</span>
  }
  if (type === 'alipay') {
    return <span className={cn(base, 'bg-sky-500')}>{badgeChar(type)}</span>
  }
  if (type === 'wxpay' || type === 'wechat') {
    return <span className={cn(base, 'bg-emerald-500')}>{badgeChar(type)}</span>
  }
  if (type === 'usdt') {
    return <span className={cn(base, 'bg-teal-600')}>{badgeChar(type)}</span>
  }
  return <span className={cn(base, 'bg-primary')}>{badgeChar(type)}</span>
}

function badgeChar(type: string): string {
  if (type === 'alipay') return '支'
  if (type === 'wxpay' || type === 'wechat') return '微'
  if (type === 'usdt') return '₮'
  return type.slice(0, 1).toUpperCase()
}
