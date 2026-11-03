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

import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import {
  formatSavingsPercent,
  getSavingsRatio,
  parseOfficialPrice,
  type OfficialPrice,
} from '../lib/adty-savings'
import type { PricingModel } from '../types'

/** Official input/output price, or the per-call price (adty task 8.5). Empty without one. */
export function OfficialPriceCell(props: { official?: OfficialPrice }) {
  const { t } = useTranslation()
  if (!props.official) return null
  const metrics: Array<{ label: string; value: number }> = []
  for (const [label, raw] of [
    [t('Input'), props.official.input],
    [t('Output'), props.official.output],
  ] as const) {
    const value = parseOfficialPrice(raw)
    if (value != null) metrics.push({ label, value })
  }
  const perCall = parseOfficialPrice(props.official.per_call)
  if (metrics.length === 0 && perCall != null) {
    metrics.push({ label: t('Per-request'), value: perCall })
  }
  return (
    <span className='grid gap-0.5'>
      {metrics.map((metric) => (
        <span key={metric.label} className='flex items-baseline gap-1.5'>
          <span className='text-muted-foreground text-xs'>{metric.label}</span>
          <span className='font-mono text-sm tabular-nums'>
            {formatBillingCurrencyFromUSD(metric.value, {
              digitsLarge: 4,
              digitsSmall: 6,
              abbreviate: false,
            })}
          </span>
        </span>
      ))}
    </span>
  )
}

/** Savings against the official price; empty without one, a dash when negative. */
export function SavingsCell(props: {
  model: PricingModel
  official?: OfficialPrice
  selectedGroup?: string
}) {
  const { i18n } = useTranslation()
  const ratio = getSavingsRatio(
    props.model,
    props.official,
    props.selectedGroup
  )
  if (ratio == null) return null
  if (ratio < 0) return <span className='text-muted-foreground'>—</span>
  return (
    <span className='text-success font-mono text-sm font-medium tabular-nums'>
      {formatSavingsPercent(
        ratio,
        toIntlLocale(i18n.resolvedLanguage || i18n.language)
      )}
    </span>
  )
}
