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
import { useTranslation } from 'react-i18next'

import { formatSavingsPercent } from '@/features/pricing/lib/adty-savings'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { cn } from '@/lib/utils'

import type { LandingPrice } from '../lib/landing-data'

const PRICE_FORMAT = { digitsLarge: 4, digitsSmall: 6, abbreviate: false }

/** "$3 / $15" per 1M tokens, or "$0.04 / call". */
export function PriceText(props: { price: LandingPrice; className?: string }) {
  const { t } = useTranslation()
  if (props.price.kind === 'per_call') {
    return (
      <span className={cn('font-mono tabular-nums', props.className)}>
        {t('{{price}} / call', {
          price: formatBillingCurrencyFromUSD(props.price.value, PRICE_FORMAT),
        })}
      </span>
    )
  }
  return (
    <span className={cn('font-mono tabular-nums', props.className)}>
      {formatBillingCurrencyFromUSD(props.price.input, PRICE_FORMAT)}
      {' / '}
      {formatBillingCurrencyFromUSD(props.price.output, PRICE_FORMAT)}
    </span>
  )
}

/** Savings in green, or a dash when the site is not cheaper. */
export function SavingsText(props: { savings: number; className?: string }) {
  const { i18n } = useTranslation()
  if (props.savings <= 0) {
    return <span className='text-muted-foreground'>—</span>
  }
  return (
    <span
      className={cn(
        'text-success font-mono font-medium tabular-nums',
        props.className
      )}
    >
      {formatSavingsPercent(
        props.savings,
        toIntlLocale(i18n.resolvedLanguage || i18n.language)
      )}
    </span>
  )
}
