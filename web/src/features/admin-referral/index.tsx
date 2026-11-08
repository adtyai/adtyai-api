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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  adminReferralKeys,
  getReferralBadges,
  getReferralOverview,
  runSettlement,
} from './api'
import { AffiliatePanel } from './components/affiliate-panel'
import { SettingsPanel } from './components/settings-panel'
import { WithdrawalPanel } from './components/withdrawal-panel'

/** Referral admin (adty task 6.7): overview, pending reviews, affiliates, withdrawals, settings. */
export function AdminReferral() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canReview = canAdmin(user, ADTY_PERMISSIONS.REFERRAL_REVIEW)
  const canPayout = canAdmin(user, ADTY_PERMISSIONS.REFERRAL_PAYOUT)
  const canAdjust = canAdmin(user, ADTY_PERMISSIONS.REFERRAL_ADJUST)
  const canSettings = canAdmin(user, ADTY_PERMISSIONS.REFERRAL_SETTINGS)
  const canReadSettings = canAdmin(
    user,
    ADTY_PERMISSIONS.REFERRAL_SETTINGS_READ
  )
  const overview = useQuery({
    queryKey: adminReferralKeys.overview,
    queryFn: getReferralOverview,
  })
  const badges = useQuery({
    queryKey: adminReferralKeys.badges,
    queryFn: getReferralBadges,
  })
  const settle = useMutation({
    mutationFn: runSettlement,
    onSuccess: () => {
      toast.success(t('Settlement finished'))
      void queryClient.invalidateQueries({ queryKey: adminReferralKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const data = overview.data
  const pendingPartners = badges.data?.partners ?? 0
  const pendingWithdrawals =
    (badges.data?.pending_withdrawals ?? 0) +
    (badges.data?.approved_withdrawals ?? 0)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Referral admin')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='grid grid-cols-2 gap-3 lg:grid-cols-6'>
            {[
              {
                label: t('Affiliates'),
                value: formatNumber(data?.affiliates ?? 0, locale),
              },
              {
                label: t('Referred users'),
                value: formatNumber(data?.bindings ?? 0, locale),
              },
              {
                label: t('Settling'),
                value: formatUsd(Number(data?.pending ?? 0), locale),
              },
              {
                label: t('Withdrawable'),
                value: formatUsd(Number(data?.available ?? 0), locale),
              },
              {
                label: t('Withdrawing'),
                value: formatUsd(Number(data?.frozen ?? 0), locale),
              },
              {
                label: t('Withdrawn'),
                value: formatUsd(Number(data?.withdrawn ?? 0), locale),
              },
            ].map((item) => (
              <Card key={item.label}>
                <CardContent className='flex flex-col gap-1 py-4'>
                  <span className='text-muted-foreground text-xs'>
                    {item.label}
                  </span>
                  <span className='text-xl font-semibold'>{item.value}</span>
                </CardContent>
              </Card>
            ))}
          </div>
          <Tabs defaultValue='withdrawals'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <TabsList>
                <TabsTrigger value='withdrawals'>
                  {t('Withdrawals')}
                  {pendingWithdrawals > 0 && ` (${pendingWithdrawals})`}
                </TabsTrigger>
                <TabsTrigger value='partners'>
                  {t('Partner applications')}
                  {pendingPartners > 0 && ` (${pendingPartners})`}
                </TabsTrigger>
                <TabsTrigger value='affiliates'>{t('Affiliates')}</TabsTrigger>
                {canReadSettings && (
                  <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
                )}
              </TabsList>
              {canAdjust && (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={settle.isPending}
                  onClick={() => settle.mutate()}
                >
                  {t('Run settlement now')}
                </Button>
              )}
            </div>
            <TabsContent value='withdrawals'>
              <WithdrawalPanel canReview={canReview} canPayout={canPayout} />
            </TabsContent>
            <TabsContent value='partners'>
              <AffiliatePanel
                canReview={canReview}
                canAdjust={false}
                partnerOnly
              />
            </TabsContent>
            <TabsContent value='affiliates'>
              <AffiliatePanel canReview={canReview} canAdjust={canAdjust} />
            </TabsContent>
            {canReadSettings && (
              <TabsContent value='settings'>
                <SettingsPanel canWrite={canSettings} />
              </TabsContent>
            )}
          </Tabs>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
