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
/** Saved values of adty_seo_setting (setting/adty_setting/seo.go). */
export type AdtySeoDefaults = {
  /** JSON object: content language → home page title. */
  home_title: string
  /** JSON object: content language → home page description. */
  home_description: string
  og_image: string
}

/**
 * Option defaults for the site settings page (adty task 9.3). getOptionValue
 * only reads back keys that have a default.
 */
export const ADTY_SEO_OPTION_DEFAULTS = {
  'adty_seo_setting.home_title': '{}',
  'adty_seo_setting.home_description': '{}',
  'adty_seo_setting.og_image': '/adty-og.png',
}

export type AdtySeoOptionSettings = typeof ADTY_SEO_OPTION_DEFAULTS

export function toAdtySeoDefaults(
  settings: AdtySeoOptionSettings
): AdtySeoDefaults {
  return {
    home_title: settings['adty_seo_setting.home_title'],
    home_description: settings['adty_seo_setting.home_description'],
    og_image: settings['adty_seo_setting.og_image'],
  }
}
