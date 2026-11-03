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
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Card, CardContent } from '@/components/ui/card'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'

import type { ReferralProfile } from '../types'

/** The dark hero card: link, copy, QR code and the program terms. */
export function ReferralLinkCard(props: { profile: ReferralProfile }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const profile = props.profile
  const link = `${window.location.origin}/r/${profile.affiliate.invite_code}`

  let notice = ''
  if (profile.site_owner) {
    notice = t('You run a reseller site, so referral commission has stopped.')
  } else if (!profile.enabled) {
    notice = t('The referral program is not open yet.')
  } else if (profile.affiliate.status === 'disabled') {
    notice = t('Your referral account is disabled.')
  }

  return (
    <Card className='bg-foreground text-background'>
      <CardContent className='flex flex-col gap-6 py-6 md:flex-row md:items-center md:justify-between'>
        <div className='flex min-w-0 flex-col gap-4'>
          <div className='flex flex-col gap-1'>
            <span className='text-background/70 text-xs tracking-wider uppercase'>
              {t('Your referral link')}
            </span>
            <div className='flex items-center gap-2'>
              <span
                className='truncate font-mono text-lg'
                data-testid='referral-link'
              >
                {link}
              </span>
              <CopyButton
                value={link}
                variant='secondary'
                size='icon'
                aria-label={t('Copy link')}
              />
            </div>
          </div>
          {notice && <p className='text-warning text-sm'>{notice}</p>}
          <dl className='grid grid-cols-2 gap-4 text-sm sm:grid-cols-3'>
            <div>
              <dt className='text-background/70 text-xs'>
                {t('Commission rate')}
              </dt>
              <dd className='text-xl font-semibold'>
                {profile.rate
                  ? t('{{rate}}%, for life', { rate: profile.rate })
                  : t('Not set')}
              </dd>
            </div>
            <div>
              <dt className='text-background/70 text-xs'>
                {t('Settles after')}
              </dt>
              <dd className='text-xl font-semibold'>
                {t('{{days}} days', { days: profile.freeze_days })}
              </dd>
            </div>
            <div>
              <dt className='text-background/70 text-xs'>
                {t('Minimum withdrawal')}
              </dt>
              <dd className='text-xl font-semibold'>
                {profile.min_withdraw_usd === null
                  ? t('Not set')
                  : formatUsd(profile.min_withdraw_usd, locale)}
              </dd>
            </div>
          </dl>
        </div>
        <div className='bg-background shrink-0 self-start rounded-lg p-3'>
          <QRCodeSVG value={link} size={120} aria-label={t('QR code')} />
        </div>
      </CardContent>
    </Card>
  )
}
