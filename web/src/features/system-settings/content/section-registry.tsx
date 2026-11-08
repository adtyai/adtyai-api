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
import type { ContentSettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'
import { AdtyLegalSection } from './adty-legal-section'
import { AnnouncementsSection } from './announcements-section'
import { ApiInfoSection } from './api-info-section'
import { ChatSettingsSection } from './chat-settings-section'
import { DashboardSection } from './dashboard-section'
import { DrawingSettingsSection } from './drawing-settings-section'
import { FAQSection } from './faq-section'
import { UptimeKumaSection } from './uptime-kuma-section'

/**
 * Validate and coerce DataExportDefaultTime to a safe value
 */
function validateDataExportDefaultTime(value: string): 'week' | 'hour' | 'day' {
  if (value === 'week' || value === 'hour' || value === 'day') {
    return value
  }
  // Default to 'hour' if value is unexpected
  return 'hour'
}

const CONTENT_SECTIONS = [
  {
    id: 'dashboard',
    titleKey: 'Data Dashboard',
    build: (settings: ContentSettings) => (
      <DashboardSection
        defaultValues={{
          DataExportEnabled: settings.DataExportEnabled,
          DataExportInterval: settings.DataExportInterval,
          DataExportDefaultTime: validateDataExportDefaultTime(
            settings.DataExportDefaultTime
          ),
        }}
      />
    ),
  },
  {
    id: 'announcements',
    titleKey: 'Announcements',
    build: (settings: ContentSettings) => (
      <AnnouncementsSection
        enabled={settings['console_setting.announcements_enabled']}
        data={settings['console_setting.announcements']}
      />
    ),
  },
  {
    id: 'api-info',
    titleKey: 'API Addresses',
    build: (settings: ContentSettings) => (
      <ApiInfoSection
        enabled={settings['console_setting.api_info_enabled']}
        data={settings['console_setting.api_info']}
      />
    ),
  },
  {
    id: 'faq',
    titleKey: 'FAQ',
    build: (settings: ContentSettings) => (
      <FAQSection
        enabled={settings['console_setting.faq_enabled']}
        data={settings['console_setting.faq']}
      />
    ),
  },
  {
    id: 'uptime-kuma',
    titleKey: 'Uptime Kuma',
    build: (settings: ContentSettings) => (
      <UptimeKumaSection
        enabled={settings['console_setting.uptime_kuma_enabled']}
        data={settings['console_setting.uptime_kuma_groups']}
      />
    ),
  },
  {
    id: 'chat',
    titleKey: 'Chat Presets',
    build: (settings: ContentSettings) => (
      <ChatSettingsSection defaultValue={settings.Chats} />
    ),
  },
  {
    id: 'drawing',
    titleKey: 'Drawing',
    build: (settings: ContentSettings) => (
      <DrawingSettingsSection
        defaultValues={{
          DrawingEnabled: settings.DrawingEnabled,
          MjNotifyEnabled: settings.MjNotifyEnabled,
          MjAccountFilterEnabled: settings.MjAccountFilterEnabled,
          MjForwardUrlEnabled: settings.MjForwardUrlEnabled,
          MjModeClearEnabled: settings.MjModeClearEnabled,
          MjActionCheckSuccessEnabled: settings.MjActionCheckSuccessEnabled,
        }}
      />
    ),
  },
  {
    id: 'adty-legal',
    titleKey: 'Multilingual Legal Pages',
    build: (settings: ContentSettings) => (
      <AdtyLegalSection
        defaultValues={{
          user_agreement: settings['adty_legal_setting.user_agreement'],
          privacy_policy: settings['adty_legal_setting.privacy_policy'],
          refund_policy: settings['adty_legal_setting.refund_policy'],
          site_agreement: settings['adty_legal_setting.site_agreement'],
          site_customer_terms:
            settings['adty_legal_setting.site_customer_terms'],
          site_agreement_version:
            settings['adty_legal_setting.site_agreement_version'],
          site_customer_terms_version:
            settings['adty_legal_setting.site_customer_terms_version'],
        }}
      />
    ),
  },
] as const

export type ContentSectionId = (typeof CONTENT_SECTIONS)[number]['id']

const contentRegistry = createSectionRegistry<
  ContentSectionId,
  ContentSettings
>({
  sections: CONTENT_SECTIONS,
  defaultSection: 'dashboard',
  basePath: '/system-settings/content',
  urlStyle: 'path',
})

export const CONTENT_SECTION_IDS = contentRegistry.sectionIds
export const CONTENT_DEFAULT_SECTION = contentRegistry.defaultSection
export const getContentSectionNavItems = contentRegistry.getSectionNavItems
export const getContentSectionContent = contentRegistry.getSectionContent
export const getContentSectionMeta = contentRegistry.getSectionMeta
