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
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { TeamRegistrationLinksPanel } from '@/features/adty-team-registration'
import { DraftNavigationProvider } from '@/features/system-settings/components/draft-navigation-provider'
import { USER_ROLE } from '@/features/users/constants'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { BusinessTimezonePanel } from './components/business-timezone-panel'
import { MainTeamPricePanel } from './components/main-team-price-panel'
import { OverviewPanel } from './components/overview-panel'
import { PausedSiteRepairPanel } from './components/paused-site-repair-panel'
import { ReviewsPanel } from './components/reviews-panel'
import { SettingsPanel } from './components/settings-panel'
import { SiteTiersPanel } from './components/site-tiers-panel'
import { SiteWithdrawalsPanel } from './components/site-withdrawals-panel'
import { VoucherAdminPanel } from './components/voucher-admin-panel'
import { SiteDetailSheet } from './components/site-detail-sheet'
import { SiteReferralCapPanel } from './components/site-referral-cap-panel'
import { SitesPanel } from './components/sites-panel'
import { UsagePanel } from './components/usage-panel'
import { WithdrawRulesPanel } from './components/withdraw-rules-panel'
import {
  canOpenSiteAdminSection,
  SITE_ADMIN_SECTIONS,
  type SiteAdminSectionId,
} from './sections'

/** Main site's reseller site management (task 11.9). */
export function SiteAdmin(props: { section: SiteAdminSectionId }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.auth.user)
  const [openSite, setOpenSite] = useState<number | null>(null)
  const canSettings = canAdmin(user, ADTY_PERMISSIONS.SITE_SETTINGS)
  const canWriteSecrets = user?.role === 100
  const labels: Record<SiteAdminSectionId, string> = {
    sites: t('Sites'),
    overview: t('Overview'),
    reviews: t('Risk reviews and reports'),
    settings: t('Settings'),
    withdrawals: t('Withdrawals'),
    vouchers: t('Recharge codes'),
  }
  const visible = SITE_ADMIN_SECTIONS.filter((section) =>
    canOpenSiteAdminSection(user, section)
  )

  return (
    <DraftNavigationProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Reseller sites')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(
            'Open sites, approve or reject applications, review flagged customers and reports.'
          )}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          {props.section === 'sites' && <PausedSiteRepairPanel />}
          <Tabs
            value={props.section}
            onValueChange={(value) =>
              void navigate({
                to: '/site-admin/$section',
                params: { section: value as SiteAdminSectionId },
              })
            }
          >
            <TabsList
              variant='line'
              className='border-border h-auto flex-wrap justify-start gap-x-6 gap-y-1 rounded-none border-b p-0 group-data-horizontal/tabs:h-auto [--tab-underline:var(--brand)]'
            >
              {visible.map((section) => (
                <TabsTrigger
                  key={section.id}
                  value={section.id}
                  className='text-dim hover:text-foreground data-active:text-foreground h-auto flex-none px-0 pt-0 pb-2.5 text-[13.5px] font-medium'
                >
                  {labels[section.id]}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value='sites' className='min-w-0'>
              <SitesPanel onOpenSite={(site) => setOpenSite(site.id)} />
            </TabsContent>
            <TabsContent value='overview'>
              <OverviewPanel />
            </TabsContent>
            <TabsContent value='reviews'>
              <ReviewsPanel />
            </TabsContent>
            <TabsContent value='settings' className='flex flex-col gap-5'>
              <SiteReferralCapPanel />
              <BusinessTimezonePanel />
              <WithdrawRulesPanel />
              <SiteTiersPanel />
              {user?.role === USER_ROLE.ROOT && (
                <TeamRegistrationLinksPanel actor='root' />
              )}
              <MainTeamPricePanel />
              {canAdmin(user, ADTY_PERMISSIONS.SITE_READ) && <UsagePanel />}
              <SettingsPanel
                canWrite={canSettings}
                canWriteSecrets={canWriteSecrets}
              />
            </TabsContent>
            <TabsContent value='withdrawals'>
              <SiteWithdrawalsPanel />
            </TabsContent>
            <TabsContent value='vouchers'>
              <VoucherAdminPanel />
            </TabsContent>
          </Tabs>
          <SiteDetailSheet
            siteId={openSite}
            onOpenChange={(open) => !open && setOpenSite(null)}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    </DraftNavigationProvider>
  )
}
