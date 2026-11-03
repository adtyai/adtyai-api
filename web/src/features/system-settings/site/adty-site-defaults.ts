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
/** Saved values of adty_site_setting (setting/adty_setting/site.go). */
export type AdtySiteDefaults = {
  api_base_url: string
  support_email: string
  telegram_url: string
  support_hours: string
  tawk_property_id: string
  tawk_widget_id: string
  refund_policy_enabled: boolean
}

/**
 * Option defaults for the site settings page (adty task 8.3). getOptionValue
 * only reads back keys that have a default, so without these the form is
 * always empty.
 */
export const ADTY_SITE_OPTION_DEFAULTS = {
  'adty_site_setting.api_base_url': '',
  'adty_site_setting.support_email': '',
  'adty_site_setting.telegram_url': '',
  'adty_site_setting.support_hours': '',
  'adty_site_setting.tawk_property_id': '',
  'adty_site_setting.tawk_widget_id': '',
  'adty_site_setting.refund_policy_enabled': false,
}

export type AdtySiteOptionSettings = typeof ADTY_SITE_OPTION_DEFAULTS

export function toAdtySiteDefaults(
  settings: AdtySiteOptionSettings
): AdtySiteDefaults {
  return {
    api_base_url: settings['adty_site_setting.api_base_url'],
    support_email: settings['adty_site_setting.support_email'],
    telegram_url: settings['adty_site_setting.telegram_url'],
    support_hours: settings['adty_site_setting.support_hours'],
    tawk_property_id: settings['adty_site_setting.tawk_property_id'],
    tawk_widget_id: settings['adty_site_setting.tawk_widget_id'],
    refund_policy_enabled: settings['adty_site_setting.refund_policy_enabled'],
  }
}
