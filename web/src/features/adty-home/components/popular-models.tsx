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

import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ADTY_BRAND_NAME } from '@/lib/adty-brand'

import {
  POPULAR_MODEL_LIMIT,
  type LandingPrices,
} from '../hooks/use-landing-data'
import { LandingSection } from './landing-section'
import { PriceText, SavingsText } from './price-text'

/** Home page price table: official price, our price and the savings. */
export function PopularModels(props: { prices: LandingPrices }) {
  const { t } = useTranslation()
  const prices = props.prices

  // Literal t() calls so the i18n scanner sees every message.
  let message: string | null = null
  if (prices.state === 'unavailable' && prices.reason === 'sign-in') {
    message = t('Sign in to see model prices.')
  } else if (prices.state === 'unavailable' && prices.reason === 'disabled') {
    message = t('Model prices are not public on this site.')
  } else if (prices.state === 'unavailable') {
    message = t('Model prices could not be loaded. Please try again later.')
  } else if (prices.state === 'ready' && prices.rows.length === 0) {
    message = t('No model has an official price to compare yet.')
  }

  return (
    <LandingSection
      id='popular-models'
      title={t('Popular model prices')}
      description={t(
        'USD per 1M tokens (input / output), or per call. Billed by usage; signed-in users see the price of their own group.'
      )}
      action={
        <a
          href='/pricing'
          className='text-sm font-medium underline-offset-4 hover:underline'
        >
          {t('View all models')} →
        </a>
      }
    >
      {message ? (
        <p className='text-muted-foreground rounded-xl border px-4 py-10 text-center text-sm'>
          {message}
        </p>
      ) : (
        <div className='rounded-xl border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Vendor')}</TableHead>
                <TableHead className='text-right'>
                  {t('Official price')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('{{site}} price', { site: ADTY_BRAND_NAME })}
                </TableHead>
                <TableHead className='text-right'>{t('Savings')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {prices.state === 'ready'
                ? prices.rows.map((row) => (
                    <TableRow key={row.model}>
                      <TableCell className='font-mono'>{row.model}</TableCell>
                      <TableCell className='text-muted-foreground'>
                        {row.vendor || '—'}
                      </TableCell>
                      <TableCell className='text-right'>
                        <PriceText
                          price={row.official}
                          className='text-muted-foreground line-through'
                        />
                      </TableCell>
                      <TableCell className='text-right'>
                        <PriceText price={row.site} />
                      </TableCell>
                      <TableCell className='text-right'>
                        <SavingsText savings={row.savings} />
                      </TableCell>
                    </TableRow>
                  ))
                : Array.from({ length: POPULAR_MODEL_LIMIT }, (_, index) => (
                    <TableRow key={index} aria-hidden>
                      <TableCell colSpan={5}>
                        <Skeleton className='h-5 w-full' />
                      </TableCell>
                    </TableRow>
                  ))}
            </TableBody>
          </Table>
        </div>
      )}
    </LandingSection>
  )
}
