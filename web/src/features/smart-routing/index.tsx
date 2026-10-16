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

import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CostPage } from './pages/cost-page'
import { DecisionsPage } from './pages/decisions-page'
import { RankingsPage } from './pages/rankings-page'
import { ScoresPage } from './pages/scores-page'
import { StrategyPage } from './pages/strategy-page'
import {
  getSmartRoutingSectionTitleKey,
  SMART_ROUTING_SECTIONS,
  type SmartRoutingSectionId,
} from './section-registry'

const route = getRouteApi('/_authenticated/smart-routing/$section')

export function SmartRouting() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = route.useParams()
  const search = route.useSearch()
  const section = params.section as SmartRoutingSectionId

  const handleSectionChange = useCallback(
    (next: string) => {
      void navigate({
        to: '/smart-routing/$section',
        params: { section: next },
        search: {},
      })
    },
    [navigate]
  )

  let body: React.ReactNode
  if (section === 'strategy') {
    body = <StrategyPage />
  } else if (section === 'scores') {
    body = <ScoresPage initialChannelId={search.channel} />
  } else if (section === 'rankings') {
    body = <RankingsPage />
  } else if (section === 'cost') {
    body = <CostPage />
  } else {
    body = <DecisionsPage />
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {t(getSmartRoutingSectionTitleKey(section))}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          <Tabs value={section} onValueChange={handleSectionChange}>
            <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
              {SMART_ROUTING_SECTIONS.map((s) => (
                <TabsTrigger key={s.id} value={s.id}>
                  {t(s.titleKey)}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <div className='min-h-0 flex-1'>{body}</div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
