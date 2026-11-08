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
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { InviteCodesPanel } from './components/invite-codes-panel'
import { OverviewPanel } from './components/overview-panel'
import { ReviewsPanel } from './components/reviews-panel'
import { SettingsPanel } from './components/settings-panel'
import { SiteDetailSheet } from './components/site-detail-sheet'
import { SitesPanel } from './components/sites-panel'
import { UsagePanel } from './components/usage-panel'
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
    'invite-codes': t('Invite codes'),
    settings: t('Settings'),
  }
  const visible = SITE_ADMIN_SECTIONS.filter((section) =>
    canOpenSiteAdminSection(user, section)
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Reseller sites')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Tabs
          value={props.section}
          onValueChange={(value) =>
            void navigate({
              to: '/site-admin/$section',
              params: { section: value as SiteAdminSectionId },
            })
          }
        >
          <TabsList>
            {visible.map((section) => (
              <TabsTrigger key={section.id} value={section.id}>
                {labels[section.id]}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value='sites'>
            <SitesPanel onOpenSite={(site) => setOpenSite(site.id)} />
          </TabsContent>
          <TabsContent value='overview'>
            <OverviewPanel />
          </TabsContent>
          <TabsContent value='reviews'>
            <ReviewsPanel />
          </TabsContent>
          <TabsContent value='invite-codes'>
            <InviteCodesPanel
              canCreate={canAdmin(user, ADTY_PERMISSIONS.SITE_SETTINGS_CREATE)}
              canWrite={canSettings}
              canDelete={canAdmin(user, ADTY_PERMISSIONS.SITE_SETTINGS_DELETE)}
            />
          </TabsContent>
          <TabsContent value='settings' className='flex flex-col gap-6'>
            {canAdmin(user, ADTY_PERMISSIONS.SITE_READ) && <UsagePanel />}
            <SettingsPanel
              canWrite={canSettings}
              canWriteSecrets={canWriteSecrets}
            />
          </TabsContent>
        </Tabs>
        <SiteDetailSheet
          siteId={openSite}
          onOpenChange={(open) => !open && setOpenSite(null)}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
