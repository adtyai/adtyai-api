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

import { paymentMethodLabels } from '../lib/payment-methods'
import { LandingSection } from './landing-section'

/** Three steps to the first request. Client guides are the next section. */
export function Steps(props: { paymentMethodIds?: string[] }) {
  const { t } = useTranslation()
  const methods = paymentMethodLabels(props.paymentMethodIds, t)
  const methodText =
    methods == null
      ? t('the payment methods currently enabled')
      : methods.length === 0
        ? t('No payment methods available. Please contact administrator.')
        : methods.join(', ')
  const steps = [
    {
      title: t('Create an account'),
      body: t('Sign up with your email address. No phone number needed.'),
    },
    {
      title: t('Top up and create a key'),
      body: t(
        'Top up with {{methods}}, then create an API key in the console.',
        { methods: methodText }
      ),
    },
    {
      title: t('Change the base_url'),
      body: t(
        'Point your client or SDK at our address. OpenAI, Anthropic and Gemini requests keep their format.'
      ),
    },
  ]

  return (
    <LandingSection
      id='steps'
      title={t('Start in three steps')}
      description={t('No code changes: swap the base_url and the key.')}
    >
      <ol className='grid gap-4 md:grid-cols-3'>
        {steps.map((step, index) => (
          <li key={step.title} className='bg-card rounded-xl border p-5'>
            <span
              className='bg-primary text-primary-foreground mb-4 inline-flex size-6 items-center justify-center rounded-md text-xs font-semibold'
              aria-hidden
            >
              {index + 1}
            </span>
            <h3 className='mb-1.5 text-sm font-semibold'>{step.title}</h3>
            <p className='text-muted-foreground text-sm'>{step.body}</p>
          </li>
        ))}
      </ol>
    </LandingSection>
  )
}
