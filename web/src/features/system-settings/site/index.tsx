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
import { ADTY_BRAND_NAME } from '@/lib/adty-brand'

import { SettingsPage } from '../components/settings-page'
import type { SiteSettings } from '../types'
import { ADTY_SEO_OPTION_DEFAULTS } from './adty-seo-defaults'
import { ADTY_SITE_OPTION_DEFAULTS } from './adty-site-defaults'
import {
  SITE_DEFAULT_SECTION,
  getSiteSectionContent,
  getSiteSectionMeta,
} from './section-registry.tsx'

const defaultSiteSettings: SiteSettings = {
  Notice: '',
  SystemName: ADTY_BRAND_NAME,
  Logo: '',
  Footer: '',
  About: '',
  HomePageContent: '',
  ServerAddress: '',
  TaskPublicAddress: '',
  'general_setting.docs_link': '',
  'legal.user_agreement': '',
  'legal.privacy_policy': '',
  HeaderNavModules: '',
  SidebarModulesAdmin: '',
  ...ADTY_SITE_OPTION_DEFAULTS,
  ...ADTY_SEO_OPTION_DEFAULTS,
}

export function SiteSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/site/$section'
      defaultSettings={defaultSiteSettings}
      defaultSection={SITE_DEFAULT_SECTION}
      getSectionContent={getSiteSectionContent}
      getSectionMeta={getSiteSectionMeta}
    />
  )
}
