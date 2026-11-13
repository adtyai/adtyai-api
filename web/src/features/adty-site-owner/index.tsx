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
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { RechargeAudit } from '@/features/recharge-audit'
import { DraftNavigationProvider } from '@/features/system-settings/components/draft-navigation-provider'
import { useAuthStore } from '@/stores/auth-store'

import { getOwnerSite, siteOwnerKeys } from './api'
import { AgreementPanel } from './components/agreement-panel'
import { BonusPanel } from './components/bonus-panel'
import { CustomersPanel } from './components/customers-panel'
import { SiteReferralPanel } from './components/site-referral-panel'
import { SiteTeamDiscountPanel } from './components/site-team-discount-panel'
import { EarningToMarginPanel } from './components/earning-to-margin-panel'
import { EarningsPanel } from './components/earnings-panel'
import { SettlementSheetPanel } from './components/settlement-sheet-panel'
import { WithdrawalPanel } from './components/withdrawal-panel'
import { OverviewPanel } from './components/overview-panel'
import { OwnerVerificationProvider } from './components/owner-verification'
import { PricingPanel } from './components/pricing-panel'
import { PromoCodesPanel } from './components/promo-codes-panel'
import { SiteSettingsPanel } from './components/site-settings-panel'
import { StaffPanel } from './components/staff-panel'
import { TermsAcceptPanel } from './components/terms-accept-panel'
import { TelegramBindCard } from '@/features/adty-telegram/bind-card'

import { VoucherPanel } from './components/voucher-panel'
import { SITE_OWNER_SECTIONS, type SiteOwnerSectionId } from './sections'

/** Site owner center (task 11.6): one page with a tab per section. */
export function SiteOwnerCenter(props: { section: SiteOwnerSectionId }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.auth.user)
  const termsRequired = user?.site_terms_required === true
  const site = useQuery({
    queryKey: siteOwnerKeys.site,
    queryFn: getOwnerSite,
    enabled: !termsRequired,
  })
  const labels: Record<SiteOwnerSectionId, string> = {
    overview: t('Overview'),
    customers: t('Customers'),
    staff: t('Staff accounts'),
    orders: t('Order management'),
    pricing: t('Pricing'),
    bonus: t('Bonuses'),
    'promo-codes': t('Promo codes'),
    settings: t('Site settings'),
    earnings: t('Earnings'),
    vouchers: t('Recharge codes'),
    notices: t('Notices'),
    agreement: t('Site agreement'),
  }

  return (
    <OwnerVerificationProvider>
      <DraftNavigationProvider>
        <SectionPageLayout>
          <SectionPageLayout.Title>
            {t('Site owner center')}
          </SectionPageLayout.Title>
          <SectionPageLayout.Content>
            {termsRequired && (
              <TermsAcceptPanel version={user?.site_terms_version ?? ''} />
            )}
            {!termsRequired && site.isLoading && (
              <Skeleton className='h-40 w-full' />
            )}
            {!termsRequired && site.isError && (
              <ErrorState
                className='min-h-48'
                onRetry={() => void site.refetch()}
              />
            )}
            {site.data && (
              <div className='flex flex-col gap-4'>
                {site.data.status !== 'active' && (
                  <Alert>
                    <AlertDescription>
                      {site.data.status === 'paused'
                        ? t(
                            'Your site is paused. Customers can still use it, but prices and bonuses cannot be changed and earnings are not settled.'
                          )
                        : t(
                            'Your site is closing. Customers will be emailed before it closes.'
                          )}
                    </AlertDescription>
                  </Alert>
                )}
                <Tabs
                  value={props.section}
                  onValueChange={(value) =>
                    void navigate({
                      to: '/site-owner/$section',
                      params: { section: value as SiteOwnerSectionId },
                    })
                  }
                >
                  <TabsList className='flex-wrap gap-1 group-data-horizontal/tabs:h-auto'>
                    {SITE_OWNER_SECTIONS.map((section) => (
                      <TabsTrigger key={section} value={section}>
                        {labels[section]}
                      </TabsTrigger>
                    ))}
                  </TabsList>
                  <TabsContent value='overview'>
                    <OverviewPanel />
                  </TabsContent>
                  <TabsContent value='customers' className='flex flex-col gap-8'>
                    <CustomersPanel />
                    <SiteTeamDiscountPanel />
                    <SiteReferralPanel />
                  </TabsContent>
                  <TabsContent value='orders'>
                    <RechargeAudit ownerMode ownerSiteId={site.data.id} />
                  </TabsContent>
                  <TabsContent value='pricing'>
                    <PricingPanel site={site.data} />
                  </TabsContent>
                  <TabsContent value='bonus'>
                    <BonusPanel site={site.data} />
                  </TabsContent>
                  <TabsContent value='promo-codes'>
                    <PromoCodesPanel />
                  </TabsContent>
                  <TabsContent value='staff'>
                    <StaffPanel siteId={site.data.id} />
                  </TabsContent>
                  <TabsContent value='settings'>
                    <SiteSettingsPanel site={site.data} />
                  </TabsContent>
                  <TabsContent value='earnings'>
                    <div className='flex flex-col gap-8'>
                      <EarningsPanel />
                      <SettlementSheetPanel />
                      <EarningToMarginPanel />
                      <WithdrawalPanel />
                    </div>
                  </TabsContent>
                  <TabsContent value='vouchers'>
                    <VoucherPanel />
                  </TabsContent>
                  <TabsContent value='notices'>
                    <TelegramBindCard purpose='admin' />
                  </TabsContent>
                  <TabsContent value='agreement'>
                    <AgreementPanel site={site.data} />
                  </TabsContent>
                </Tabs>
              </div>
            )}
          </SectionPageLayout.Content>
        </SectionPageLayout>
      </DraftNavigationProvider>
    </OwnerVerificationProvider>
  )
}
