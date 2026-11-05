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

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'

import { paymentMethodLabels } from '../lib/payment-methods'

import { LandingSection } from './landing-section'

/** Common questions. Closed answers stay mounted so the prerendered page has them. */
export function Faq(props: { paymentMethodIds?: string[] }) {
  const { t } = useTranslation()
  const methods = paymentMethodLabels(props.paymentMethodIds, t)
  const methodText =
    methods == null
      ? t('the payment methods currently enabled')
      : methods.length === 0
        ? t('No payment methods available. Please contact administrator.')
        : methods.join(', ')
  const items = [
    {
      id: 'difference',
      question: t('How is this different from the official API?'),
      answer: t(
        'Requests and responses use the official formats. You use one key for every model, pay with {{methods}}, and most models cost less than the official price. The pricing page lists both prices for each model.',
        { methods: methodText }
      ),
    },
    {
      id: 'payment',
      question: t('Which payment methods are supported?'),
      answer: t(
        '{{methods}}. A top-up is added to your balance in USD and is used up by your requests.',
        { methods: methodText }
      ),
    },
    {
      id: 'coding-tools',
      question: t('Can I use Claude Code and Codex CLI?'),
      answer: t(
        'Yes. Set the base URL and your API key as shown in the setup guides; no other changes are needed.'
      ),
    },
    {
      id: 'models',
      question: t('Which models are available?'),
      answer: t(
        'The pricing page lists every model you can call, with our price and the official price.'
      ),
    },
    {
      id: 'affiliate',
      question: t('How does the affiliate program work?'),
      answer: t(
        'Share your referral link. Each time someone you invited tops up, you earn a commission, for as long as they stay.'
      ),
    },
  ]

  return (
    <LandingSection
      id='faq'
      title={t('Frequently asked questions')}
      description={t('More answers are in the docs, or contact support.')}
    >
      <Accordion className='rounded-xl border px-4'>
        {items.map((item) => (
          <AccordionItem key={item.id} value={item.id}>
            <AccordionTrigger className='py-4'>
              {item.question}
            </AccordionTrigger>
            <AccordionContent keepMounted className='text-muted-foreground'>
              {item.answer}
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </LandingSection>
  )
}
