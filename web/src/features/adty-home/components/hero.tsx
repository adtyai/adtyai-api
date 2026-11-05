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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatSavingsPercent } from '@/features/pricing/lib/adty-savings'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { paymentMethodSentence } from '../lib/payment-methods'
import { cn } from '@/lib/utils'

import type { LandingPrices } from '../hooks/use-landing-data'
import { CodeCard } from './code-card'
import { PriceCompareCard } from './price-compare-card'

/** One figure of the stats strip; a skeleton while its data loads. */
function Stat(props: { value: ReactNode | null; label: string }) {
  return (
    // The label comes first in the markup (dt before dd); the column is
    // reversed so the figure sits on top.
    <div className='flex flex-col-reverse gap-1'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='text-2xl font-semibold tracking-tight tabular-nums md:text-3xl'>
        {props.value ?? <Skeleton className='h-8 w-16' aria-hidden />}
      </dd>
    </div>
  )
}

/**
 * First screen: headline, buttons and the stats strip on the left, the code
 * card with the official price card on the right (design §8.1).
 */
export function Hero(props: {
  signedIn: boolean
  baseUrl: string
  prices: LandingPrices
  /** USD; undefined while loading, null when not set. */
  minTopUp: number | null | undefined
  paymentMethodIds?: string[]
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const prices = props.prices
  const ready = prices.state === 'ready' ? prices : null
  const loading = prices.state === 'loading'

  // A figure that can never be filled in (no prices for this viewer, no
  // cheaper model, no minimum set) is left out instead of showing a guess.
  const stats: { key: string; value: ReactNode | null; label: string }[] = []
  if (ready || loading) {
    stats.push({
      key: 'models',
      value: ready ? formatNumber(ready.modelCount, locale) : null,
      label: t('Models available'),
    })
  }
  if (props.minTopUp !== null && props.minTopUp !== 0) {
    stats.push({
      key: 'min-topup',
      value:
        props.minTopUp === undefined
          ? null
          : formatBillingCurrencyFromUSD(props.minTopUp),
      label: t('Minimum top-up'),
    })
  }
  if (loading || ready?.best) {
    stats.push({
      key: 'savings',
      value: ready?.best ? (
        <span className='text-success'>
          {formatSavingsPercent(ready.best.savings, locale)}
        </span>
      ) : null,
      label: t('Up to this much below official prices'),
    })
  }

  return (
    <section
      id='hero'
      aria-labelledby='hero-title'
      className='mx-auto grid max-w-6xl items-center gap-12 px-4 pt-28 pb-16 md:px-6 lg:grid-cols-[1.1fr_1fr] lg:pt-32 lg:pb-20'
    >
      <div className='space-y-6'>
        <p className='text-muted-foreground inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs'>
          <span className='bg-success size-1.5 rounded-full' aria-hidden />
          {t('Compatible with the OpenAI, Anthropic and Gemini APIs')}
        </p>
        <h1
          id='hero-title'
          className='text-4xl font-semibold tracking-tight text-balance md:text-5xl'
        >
          {t('Claude, GPT and Gemini with one API key')}
        </h1>
        <p className='text-muted-foreground max-w-xl text-base text-pretty'>
          {t(
            'Change one base_url and Claude Code, Codex CLI and the OpenAI SDK keep working. Pay as you go, below official prices.'
          )}{' '}
          {paymentMethodSentence(props.paymentMethodIds, t)}
        </p>
        <div className='flex flex-wrap gap-3'>
          {props.signedIn ? (
            <a href='/dashboard' className={buttonVariants({ size: 'lg' })}>
              {t('Go to console')}
            </a>
          ) : (
            <a href='/sign-up' className={buttonVariants({ size: 'lg' })}>
              {t('Sign up')}
            </a>
          )}
          <a
            href='/pricing'
            className={buttonVariants({ size: 'lg', variant: 'outline' })}
          >
            {t('View pricing')}
          </a>
        </div>
        {stats.length > 0 && (
          <dl
            data-testid='landing-stats'
            className='flex flex-wrap gap-x-10 gap-y-4 border-t pt-6'
          >
            {stats.map((stat) => (
              <Stat key={stat.key} value={stat.value} label={stat.label} />
            ))}
          </dl>
        )}
      </div>

      <div className={cn('relative', ready?.best && 'pb-24')}>
        <CodeCard baseUrl={props.baseUrl} />
        <div className='absolute right-2 bottom-0 sm:right-6'>
          <PriceCompareCard row={ready?.best ?? null} />
        </div>
      </div>
    </section>
  )
}
