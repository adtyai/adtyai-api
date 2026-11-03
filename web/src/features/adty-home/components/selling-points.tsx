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

import { LandingSection } from './landing-section'

/** Four reasons to use the site (design §8.1). */
export function SellingPoints() {
  const { t } = useTranslation()
  const points = [
    {
      title: t('Below official prices'),
      body: t(
        'Every model lists its official price next to ours. Pay only for what you use, with no monthly fee.'
      ),
    },
    {
      title: t('Works with coding tools'),
      body: t(
        'Claude Code, Codex CLI and the OpenAI SDK work as they are. Set the base URL and the key.'
      ),
    },
    {
      title: t('Easy payment'),
      body: t('Top up with USDT, Alipay or WeChat Pay. The balance is in USD.'),
    },
    {
      title: t('Clear bills'),
      body: t(
        'Each request is logged with its model, tokens and cost, so you can check every charge.'
      ),
    },
  ]

  return (
    <LandingSection
      id='selling-points'
      title={t('Why {{site}}', { site: ADTY_BRAND_NAME })}
    >
      <ul className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        {points.map((point) => (
          <li key={point.title} className='bg-card rounded-xl border p-5'>
            <h3 className='mb-1.5 text-sm font-semibold'>{point.title}</h3>
            <p className='text-muted-foreground text-sm'>{point.body}</p>
          </li>
        ))}
      </ul>
    </LandingSection>
  )
}
