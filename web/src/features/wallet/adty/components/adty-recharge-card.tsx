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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { useDebounce } from '@/hooks/use-debounce'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { quoteAdtyTopup } from '../api'
import { bonusRateFor, bonusUsdFor } from '../lib/bonus'
import { methodLabel } from '../lib/methods'
import { formatUsd } from '../lib/money'
import type { AdtyMethod, AdtyTopupInfo } from '../types'

type AdtyRechargeCardProps = {
  info: AdtyTopupInfo | undefined
  loading: boolean
  paying: boolean
  /** Starts the payment for the chosen amount, method and promo code (P4 / P5 dialogs). */
  onPay: (amount: number, method: AdtyMethod, promoCode: string) => void
}

export function AdtyRechargeCard(props: AdtyRechargeCardProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const tiers = props.info?.tiers ?? []
  const rates = props.info?.bonus_rates ?? {}
  // No bonus tiers in the admin means no bonus at all: show plain amounts.
  const hasBonus = Object.values(rates).some((rate) => rate > 0)
  const minTopup = props.info?.min_topup ?? 1
  const [selectedTier, setSelectedTier] = useState<number | null>(null)
  const [customAmount, setCustomAmount] = useState('')
  const [methodId, setMethodId] = useState<string | null>(null)
  // adty task 12.4: the code is checked by the quote; an invalid one blocks payment.
  const [promoInput, setPromoInput] = useState('')
  const promoCode = useDebounce(promoInput.trim(), 400)

  const amount =
    selectedTier ??
    (customAmount === '' ? 0 : Number.parseInt(customAmount, 10))
  const validAmount = Number.isFinite(amount) && amount >= minTopup
  const methods = props.info?.methods ?? []
  const method = methods.find((item) => item.id === methodId) ?? null

  const quoteQuery = useQuery({
    queryKey: ['adty-topup', 'quote', amount, methodId, promoCode],
    queryFn: () => quoteAdtyTopup(amount, methodId ?? '', promoCode),
    enabled: validAmount && method !== null && method.available,
    meta: { errorToast: false },
  })
  const quote = quoteQuery.data?.quote
  const quoteAvailable = quoteQuery.data?.available ?? false
  const promoError =
    promoCode !== '' && quoteQuery.isError
      ? getServerErrorMessage(
          quoteQuery.error,
          t('This promo code is not valid')
        )
      : ''
  const promoBonus = Number(quote?.promo_bonus_usd ?? 0)

  if (props.loading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className='h-6 w-32' />
        </CardHeader>
        <CardContent className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className='h-16' />
          ))}
        </CardContent>
      </Card>
    )
  }

  const bonusRate = validAmount ? bonusRateFor(amount, rates) : 0
  const bonusUsd =
    (validAmount ? bonusUsdFor(amount, rates) : 0) +
    (promoCode !== '' && quote ? promoBonus : 0)
  let creditText = '-'
  if (validAmount && bonusUsd > 0) {
    creditText = t('{{total}} (incl. {{bonus}} bonus)', {
      total: formatUsd(amount + bonusUsd, locale),
      bonus: formatUsd(bonusUsd, locale),
    })
  } else if (validAmount) {
    creditText = formatUsd(amount, locale)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Add funds')}</CardTitle>
        {hasBonus && (
          <CardDescription>
            {t(
              'Bigger top-ups get a bigger bonus. The bonus is credited with the payment.'
            )}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className='flex flex-col gap-6'>
        <section className='flex flex-col gap-3'>
          <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
            {t('Amount')}
          </Label>
          <div className='grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-5'>
            {tiers.map((tier) => (
              <Button
                key={tier.amount}
                variant='outline'
                aria-pressed={selectedTier === tier.amount}
                className={cn(
                  'flex h-auto min-h-16 flex-col items-start gap-1 px-3 py-2.5 text-left whitespace-normal',
                  selectedTier === tier.amount
                    ? 'border-foreground bg-foreground/5'
                    : 'border-muted'
                )}
                onClick={() => {
                  setSelectedTier(tier.amount)
                  setCustomAmount('')
                }}
              >
                <span className='text-base font-semibold'>
                  ${formatNumber(tier.amount, locale)}
                </span>
                {hasBonus && (
                  <span className='text-success text-xs'>
                    {tier.rate > 0
                      ? t('Bonus {{rate}}%', { rate: tier.rate })
                      : t('No bonus')}
                  </span>
                )}
              </Button>
            ))}
          </div>
          <div className='flex flex-col gap-1.5 sm:max-w-xs'>
            <Label htmlFor='adty-custom-amount'>
              {t('Custom amount (USD)')}
            </Label>
            <Input
              id='adty-custom-amount'
              type='number'
              inputMode='numeric'
              min={minTopup}
              step={1}
              value={customAmount}
              placeholder={t('At least ${{amount}}', { amount: minTopup })}
              onChange={(event) => {
                setCustomAmount(event.target.value)
                setSelectedTier(null)
              }}
            />
            {customAmount !== '' && (hasBonus || !validAmount) && (
              <p className='text-muted-foreground text-xs'>
                {validAmount
                  ? t('Bonus {{rate}}%', { rate: bonusRate })
                  : t('At least ${{amount}}', { amount: minTopup })}
              </p>
            )}
          </div>
        </section>

        <section className='flex flex-col gap-3'>
          <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
            {t('Payment method')}
          </Label>
          <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6'>
            {methods.map((item) => (
              <Button
                key={item.id}
                variant='outline'
                disabled={!item.available}
                aria-pressed={methodId === item.id}
                className={cn(
                  'h-auto min-h-12 flex-col items-start px-3 py-2 text-left whitespace-normal',
                  methodId === item.id
                    ? 'border-foreground bg-foreground/5'
                    : 'border-muted'
                )}
                onClick={() => setMethodId(item.id)}
              >
                <span className='text-sm font-medium'>
                  {t(methodLabel(item.id))}
                </span>
                {!item.available && (
                  <span className='text-muted-foreground text-xs'>
                    {t('Unavailable')}
                  </span>
                )}
              </Button>
            ))}
          </div>
        </section>

        <section className='flex flex-col gap-1.5 sm:max-w-xs'>
          <Label htmlFor='adty-promo-code'>{t('Promo code (optional)')}</Label>
          <Input
            id='adty-promo-code'
            value={promoInput}
            maxLength={32}
            autoComplete='off'
            aria-invalid={promoError !== ''}
            onChange={(event) => setPromoInput(event.target.value)}
          />
          {promoError && (
            <p className='text-destructive text-xs'>{promoError}</p>
          )}
          {promoCode !== '' && quote && promoBonus > 0 && (
            <p className='text-success text-xs' data-testid='adty-promo-bonus'>
              {t('Promo bonus {{amount}}', {
                amount: formatUsd(promoBonus, locale),
              })}
            </p>
          )}
        </section>

        <section
          className='bg-muted/40 flex flex-col gap-2 rounded-lg border p-4'
          aria-live='polite'
        >
          <div className='flex flex-wrap items-baseline justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('You get')}
            </span>
            <span className='text-lg font-semibold' data-testid='adty-credit'>
              {creditText}
            </span>
          </div>
          {quote && (
            <div className='flex flex-wrap items-baseline justify-between gap-2'>
              <span className='text-muted-foreground text-sm'>
                {t('You pay')}
              </span>
              <span className='font-medium' data-testid='adty-pay'>
                {quote.pay_currency === 'USDT'
                  ? t('{{amount}} USDT (1 USDT = 1 USD)', {
                      amount: quote.pay_amount,
                    })
                  : t('¥{{amount}} (1 USD = {{rate}} CNY)', {
                      amount: Number(quote.pay_amount).toFixed(2),
                      rate: Number(quote.price_cny_per_usd).toFixed(2),
                    })}
              </span>
            </div>
          )}
          {quoteQuery.data && !quoteAvailable && (
            <p className='text-destructive text-sm'>
              {t(
                'This method cannot take this amount. Choose another method or amount.'
              )}
            </p>
          )}
          <Button
            className='mt-2 sm:self-end'
            disabled={
              !validAmount ||
              method === null ||
              !quoteAvailable ||
              props.paying ||
              quoteQuery.isFetching ||
              promoInput.trim() !== promoCode
            }
            onClick={() => method && props.onPay(amount, method, promoCode)}
          >
            {props.paying ? t('Processing...') : t('Pay now')}
          </Button>
        </section>
      </CardContent>
    </Card>
  )
}
