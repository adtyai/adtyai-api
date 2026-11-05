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

import { paymentMethodSentence } from '../lib/payment-methods'

/** Grey band under the hero: payment methods, protocols, support hours. */
export function PaymentStrip(props: {
  supportHours: string
  paymentMethodIds?: string[]
}) {
  const { t } = useTranslation()
  return (
    <section
      id='payment'
      aria-label={t('Payment methods')}
      className='bg-muted/50 border-y'
    >
      <ul className='text-muted-foreground mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-x-8 gap-y-2 px-4 py-4 text-xs md:px-6'>
        <li>{paymentMethodSentence(props.paymentMethodIds, t)}</li>
        <li>{t('OpenAI, Anthropic and Gemini compatible')}</li>
        {props.supportHours && (
          <li>
            {t('Support hours: {{hours}}', { hours: props.supportHours })}
          </li>
        )}
      </ul>
    </section>
  )
}
