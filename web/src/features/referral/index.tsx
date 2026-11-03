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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'

import { getReferralProfile, referralQueryKeys } from './api'
import { PartnerCard } from './components/partner-card'
import { ReferralLinkCard } from './components/referral-link-card'
import { ReferralStats } from './components/referral-stats'
import { CommissionTable, WithdrawalTable } from './components/referral-tables'
import { WithdrawDrawer } from './components/withdraw-drawer'
import type { ReferralProfile } from './types'

function ReferralRules(props: { profile: ReferralProfile }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const profile = props.profile
  const rules = [
    t(
      'You earn {{rate}}% of the top-up face value of every user who signs up through your link, for as long as they top up. Bonus credit earns nothing.',
      { rate: profile.rate || '-' }
    ),
    t(
      'Commission settles {{days}} days after the top-up. Refunded top-ups void or claw back their commission, and the balance can go below zero.',
      { days: profile.freeze_days }
    ),
    t('The last referral link a visitor opens before signing up counts.'),
    t('Inviting yourself, or many sign-ups from one network, earns nothing.'),
  ]
  if (profile.min_withdraw_usd !== null) {
    rules.push(
      t(
        'Transfers to your balance are instant and free. Other withdrawals need at least {{amount}} and are reviewed; you can cancel within {{minutes}} minutes.',
        {
          amount: formatUsd(profile.min_withdraw_usd, locale),
          minutes: profile.cancel_window_minutes,
        }
      )
    )
  }
  if (profile.usdt_withdraw_fee !== null) {
    rules.push(
      t('USDT withdrawals pay a network fee of {{fee}} USDT.', {
        fee: profile.usdt_withdraw_fee,
      })
    )
  }
  return (
    <Card>
      <CardContent className='py-4'>
        <ul className='text-muted-foreground list-disc space-y-2 pl-5 text-sm'>
          {rules.map((rule) => (
            <li key={rule}>{rule}</li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

/** The referral page: link, funnel, account, commissions, withdrawals and rules. */
export function Referral() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [withdrawing, setWithdrawing] = useState(false)
  const profileQuery = useQuery({
    queryKey: referralQueryKeys.profile,
    queryFn: getReferralProfile,
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: referralQueryKeys.all })
  const profile = profileQuery.data

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Referral')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {profileQuery.isLoading && <LoadingState />}
        {profileQuery.isError && (
          <ErrorState onRetry={() => void profileQuery.refetch()} />
        )}
        {profile && (
          <div className='flex flex-col gap-4'>
            <ReferralLinkCard profile={profile} />
            <ReferralStats
              profile={profile}
              onWithdraw={() => setWithdrawing(true)}
            />
            <PartnerCard affiliate={profile.affiliate} onApplied={refresh} />
            <Tabs defaultValue='commissions'>
              <TabsList>
                <TabsTrigger value='commissions'>
                  {t('Commission details')}
                </TabsTrigger>
                <TabsTrigger value='withdrawals'>
                  {t('Withdrawal records')}
                </TabsTrigger>
                <TabsTrigger value='rules'>{t('Program rules')}</TabsTrigger>
              </TabsList>
              <TabsContent value='commissions'>
                <CommissionTable />
              </TabsContent>
              <TabsContent value='withdrawals'>
                <WithdrawalTable
                  cancelWindowMinutes={profile.cancel_window_minutes}
                />
              </TabsContent>
              <TabsContent value='rules'>
                <ReferralRules profile={profile} />
              </TabsContent>
            </Tabs>
            <WithdrawDrawer
              open={withdrawing}
              onOpenChange={setWithdrawing}
              profile={profile}
              onDone={refresh}
            />
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
