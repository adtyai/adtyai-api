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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DraftNavigationProvider } from '@/features/system-settings/components/draft-navigation-provider'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { getMarketingSettings, marketingKeys } from './api'
import { CampaignPanel } from './components/campaign-panel'
import { PromoPanel } from './components/promo-panel'
import { SettingsPanel } from './components/settings-panel'
import { SourcesPanel } from './components/sources-panel'
import { TriggersPanel } from './components/triggers-panel'

/** Marketing admin (adty P12): campaigns, templates, automated emails and settings. */
export function AdtyMarketing() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const canCreate = canAdmin(user, ADTY_PERMISSIONS.MARKETING_CREATE)
  const canWrite = canAdmin(user, ADTY_PERMISSIONS.MARKETING_WRITE)
  const canDelete = canAdmin(user, ADTY_PERMISSIONS.MARKETING_DELETE)
  const canSend = canAdmin(user, ADTY_PERMISSIONS.MARKETING_SEND)
  const settings = useQuery({
    queryKey: marketingKeys.settings,
    queryFn: getMarketingSettings,
  })
  const includeSites = settings.data?.include_site_customers_default === true
  const [visitedTabs, setVisitedTabs] = useState(() => new Set(['campaigns']))

  return (
    <DraftNavigationProvider>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Marketing')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <Tabs
            defaultValue='campaigns'
            onValueChange={(value) =>
              setVisitedTabs((previous) => new Set(previous).add(String(value)))
            }
          >
            <TabsList>
              <TabsTrigger value='campaigns'>{t('Campaigns')}</TabsTrigger>
              <TabsTrigger value='templates'>{t('Templates')}</TabsTrigger>
              <TabsTrigger value='triggers'>
                {t('Automated emails')}
              </TabsTrigger>
              <TabsTrigger value='promo'>{t('Promo codes')}</TabsTrigger>
              <TabsTrigger value='sources'>{t('Sign-up sources')}</TabsTrigger>
              <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
            </TabsList>
            <TabsContent value='campaigns' keepMounted>
              {visitedTabs.has('campaigns') && (
                <CampaignPanel
                  templates={false}
                  canCreate={canCreate}
                  canWrite={canWrite}
                  canDelete={canDelete}
                  canSend={canSend}
                  includeSitesDefault={includeSites}
                />
              )}
            </TabsContent>
            <TabsContent value='templates' keepMounted>
              {visitedTabs.has('templates') && (
                <CampaignPanel
                  templates
                  canCreate={canCreate}
                  canWrite={canWrite}
                  canDelete={canDelete}
                  canSend={canSend}
                  includeSitesDefault={includeSites}
                />
              )}
            </TabsContent>
            <TabsContent value='triggers' keepMounted>
              {visitedTabs.has('triggers') && (
                <TriggersPanel canWrite={canWrite} />
              )}
            </TabsContent>
            <TabsContent value='promo' keepMounted>
              {visitedTabs.has('promo') && (
                <PromoPanel canCreate={canCreate} canWrite={canWrite} />
              )}
            </TabsContent>
            <TabsContent value='sources'>
              <SourcesPanel />
            </TabsContent>
            <TabsContent value='settings' keepMounted>
              {visitedTabs.has('settings') && (
                <SettingsPanel canWrite={canWrite} />
              )}
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    </DraftNavigationProvider>
  )
}
