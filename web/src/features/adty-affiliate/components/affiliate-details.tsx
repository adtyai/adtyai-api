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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ReferralPublicSettings } from '@/features/referral/types'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'

import { estimateCommission } from '../lib/estimate'

type AffiliateDetailsProps = {
  /** An open program: enabled with a default rate. */
  settings: ReferralPublicSettings & { default_rate: number }
}

/** How it works, rules, estimator and FAQ for an open affiliate program. */
export function AffiliateDetails(props: AffiliateDetailsProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const settings = props.settings
  const rate = settings.default_rate
  const [people, setPeople] = useState('10')
  const [average, setAverage] = useState('50')
  const estimate = estimateCommission(Number(people), Number(average), rate)

  const steps = [
    {
      title: t('Share your link'),
      body: t(
        'Every account gets a referral link and QR code on the Referrals page.'
      ),
    },
    {
      title: t('Friends sign up and top up'),
      body: t(
        'Anyone who signs up within {{days}} days of opening your link becomes your referral.',
        { days: settings.cookie_ttl_days }
      ),
    },
    {
      title: t('You earn on every top-up'),
      body: t(
        'You get {{rate}}% of each top-up amount, for every top-up they ever make.',
        { rate }
      ),
    },
  ]

  const rules = [
    t('Commission: {{rate}}% of the amount paid, excluding top-up bonuses.', {
      rate,
    }),
    t('Commissions become withdrawable {{days}} days after the top-up.', {
      days: settings.freeze_days,
    }),
    t('Refunded top-ups cancel or take back their commission.'),
    t(
      'Withdraw to your account balance at any time with no minimum and no fee.'
    ),
  ]
  if (settings.min_withdraw_usd !== null) {
    rules.push(
      t('Withdrawals to USDT, Alipay or WeChat Pay start at {{amount}}.', {
        amount: formatUsd(settings.min_withdraw_usd, locale),
      })
    )
  }
  if (settings.usdt_withdraw_fee !== null) {
    rules.push(
      t('USDT withdrawals have a network fee of {{fee}} USDT.', {
        fee: settings.usdt_withdraw_fee,
      })
    )
  }
  rules.push(
    t(
      'Inviting your own accounts earns nothing, and many sign-ups from one network pause your settlements for review.'
    )
  )

  const faq = [
    {
      q: t('Who can join?'),
      a: t(
        'Everyone with an account. You are enrolled automatically when you sign up.'
      ),
    },
    {
      q: t('How long do I earn from someone I invite?'),
      a: t(
        'For as long as their account exists. Each person can only be invited once, and the link they opened last before signing up counts.'
      ),
    },
    {
      q: t('Can I get a higher rate?'),
      a: t(
        'Partners with a large audience can apply for a higher rate on the Referrals page.'
      ),
    },
  ]

  return (
    <>
      <section className='grid gap-4 md:grid-cols-3'>
        {steps.map((step, index) => (
          <Card key={step.title}>
            <CardHeader>
              <p className='text-muted-foreground text-sm'>{index + 1}</p>
              <CardTitle>{step.title}</CardTitle>
            </CardHeader>
            <CardContent className='text-muted-foreground text-sm'>
              {step.body}
            </CardContent>
          </Card>
        ))}
      </section>

      <section className='grid gap-6 md:grid-cols-2'>
        <Card>
          <CardHeader>
            <CardTitle>{t('Rules')}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className='text-muted-foreground list-disc space-y-2 ps-5 text-sm'>
              {rules.map((rule) => (
                <li key={rule}>{rule}</li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('Estimate your earnings')}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-4'>
            <div className='grid grid-cols-2 gap-4'>
              <div className='space-y-2'>
                <Label htmlFor='affiliate-people'>
                  {t('People you invite')}
                </Label>
                <Input
                  id='affiliate-people'
                  type='number'
                  min={0}
                  value={people}
                  onChange={(event) => setPeople(event.target.value)}
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='affiliate-average'>
                  {t('Average monthly top-up (USD)')}
                </Label>
                <Input
                  id='affiliate-average'
                  type='number'
                  min={0}
                  value={average}
                  onChange={(event) => setAverage(event.target.value)}
                />
              </div>
            </div>
            <div className='bg-muted rounded-lg p-4'>
              <p className='text-muted-foreground text-sm'>
                {t('You would earn per month')}
              </p>
              <p
                className='text-3xl font-semibold'
                data-testid='affiliate-estimate'
              >
                {estimate === null ? '-' : formatUsd(estimate, locale)}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('People × average top-up × {{rate}}%', { rate })}
              </p>
            </div>
          </CardContent>
        </Card>
      </section>

      <section className='space-y-4'>
        <h2 className='text-2xl font-semibold'>{t('FAQ')}</h2>
        <Accordion className='w-full'>
          {faq.map((item) => (
            <AccordionItem key={item.q} value={item.q}>
              <AccordionTrigger className='text-start hover:no-underline'>
                {item.q}
              </AccordionTrigger>
              <AccordionContent className='text-muted-foreground'>
                {item.a}
              </AccordionContent>
            </AccordionItem>
          ))}
        </Accordion>
      </section>
    </>
  )
}
