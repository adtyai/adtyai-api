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

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ReferralProfile } from '../types'

type StatItem = { label: string; value: string; negative?: boolean }

function StatGrid(props: { items: StatItem[]; testId: string }) {
  return (
    <div
      className='grid grid-cols-2 gap-3 lg:grid-cols-4'
      data-testid={props.testId}
    >
      {props.items.map((item) => (
        <Card key={item.label}>
          <CardContent className='flex flex-col gap-1 py-4'>
            <span className='text-muted-foreground text-xs'>{item.label}</span>
            <span
              className={cn(
                'text-xl font-semibold',
                item.negative && 'text-destructive'
              )}
            >
              {item.value}
            </span>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

/** Funnel (clicks, sign-ups, paying, earned) and the four account balances. */
export function ReferralStats(props: {
  profile: ReferralProfile
  onWithdraw: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const funnel = props.profile.funnel
  const account = props.profile.account
  const canWithdraw =
    props.profile.enabled &&
    props.profile.affiliate.withdrawal_enabled &&
    account.available_amount > 0

  return (
    <div className='flex flex-col gap-4'>
      <StatGrid
        testId='referral-funnel'
        items={[
          { label: t('Clicks'), value: formatNumber(funnel.clicks, locale) },
          { label: t('Sign-ups'), value: formatNumber(funnel.signups, locale) },
          {
            label: t('Paying users'),
            value: formatNumber(funnel.paying, locale),
          },
          {
            label: t('Total commission'),
            value: formatUsd(Number(funnel.earned), locale),
          },
        ]}
      />
      <div className='flex items-center justify-between'>
        <h2 className='text-sm font-medium'>{t('Commission account')}</h2>
        <Button size='sm' disabled={!canWithdraw} onClick={props.onWithdraw}>
          {t('Withdraw')}
        </Button>
      </div>
      <StatGrid
        testId='referral-account'
        items={[
          {
            label: t('Settling'),
            value: formatUsd(account.pending_amount, locale),
          },
          {
            label: t('Withdrawable'),
            value: formatUsd(account.available_amount, locale),
            negative: account.available_amount < 0,
          },
          {
            label: t('Withdrawing'),
            value: formatUsd(account.frozen_amount, locale),
          },
          {
            label: t('Withdrawn'),
            value: formatUsd(account.withdrawn_amount, locale),
          },
        ]}
      />
    </div>
  )
}
