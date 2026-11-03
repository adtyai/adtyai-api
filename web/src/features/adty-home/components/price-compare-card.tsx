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

import { ADTY_BRAND_NAME } from '@/lib/adty-brand'

import type { LandingPriceRow } from '../lib/landing-data'
import { PriceText, SavingsText } from './price-text'

/**
 * Official price next to ours for the model with the largest savings, laid
 * over the code card. Renders nothing until such a model is known.
 */
export function PriceCompareCard(props: { row: LandingPriceRow | null }) {
  const { t } = useTranslation()
  const row = props.row
  if (!row) return null
  const unit = row.site.kind === 'per_call' ? t('Per call') : t('Per 1M tokens')

  return (
    <div
      data-testid='landing-price-card'
      className='bg-card text-card-foreground w-72 rounded-xl border p-4 shadow-lg'
    >
      <div className='mb-3 flex items-center justify-between gap-2'>
        <span className='truncate font-mono text-sm font-medium'>
          {row.model}
        </span>
        <span className='bg-success-soft rounded-full px-2 py-0.5 text-xs'>
          <SavingsText savings={row.savings} />
        </span>
      </div>
      <dl className='space-y-1.5 text-xs'>
        <div className='flex items-center justify-between gap-2'>
          <dt className='text-muted-foreground'>
            {t('Official')} · {unit}
          </dt>
          <dd>
            <PriceText
              price={row.official}
              className='text-muted-foreground line-through'
            />
          </dd>
        </div>
        <div className='flex items-center justify-between gap-2'>
          <dt className='text-muted-foreground'>{ADTY_BRAND_NAME}</dt>
          <dd>
            <PriceText price={row.site} className='text-sm font-semibold' />
          </dd>
        </div>
      </dl>
    </div>
  )
}
