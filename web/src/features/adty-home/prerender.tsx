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
import i18next, { type i18n as I18n } from 'i18next'
import { renderToString } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ko from '@/i18n/locales/ko.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zhCN from '@/i18n/locales/zh.json'
import type { ContentLang } from '@/lib/adty-lang'

import { LandingSections } from '.'
import { HOME_LANGS } from '../adty-seo/home-langs'
import { PrerenderHeader } from './components/prerender-header'
import type { LandingData } from './hooks/use-landing-data'

/**
 * The build has no server address; the code card shows this placeholder, in
 * the style of the key placeholder, until the page mounts and reads the
 * real one from /api/adty/site.
 */
const PRERENDER_BASE_URL = '<BASE_URL>'

// Same resources as i18n/config.ts, without its browser language detector.
const resources = { en, zhCN, fr, ru, ja, ko, vi, zhTW }

/** No server data at build time: prices and figures render as placeholders. */
const PRERENDER_DATA: LandingData = {
  prices: { state: 'loading' },
  minTopUp: undefined,
  supportHours: '',
  affiliate: { open: false, rate: null },
}

async function createI18n(interfaceLang: string): Promise<I18n> {
  const instance = i18next.createInstance()
  await instance.init({
    resources,
    lng: interfaceLang,
    fallbackLng: 'en',
    nsSeparator: false,
    interpolation: { escapeValue: false },
  })
  return instance
}

/**
 * Build-time HTML of the home page in one language (adty task 9.3): the
 * static header and the landing blocks. The footer is left out: it needs
 * the router and the status API, and the mounted page has it.
 */
export async function renderAdtyHome(lang: ContentLang): Promise<string> {
  const homeLang = HOME_LANGS.find((item) => item.contentLang === lang)
  const instance = await createI18n(homeLang?.interfaceLang ?? 'en')
  return renderToString(
    <I18nextProvider i18n={instance}>
      <PrerenderHeader />
      <LandingSections
        signedIn={false}
        baseUrl={PRERENDER_BASE_URL}
        data={PRERENDER_DATA}
      />
    </I18nextProvider>
  )
}

/** All eight languages, keyed by content language code. */
export async function renderAllAdtyHomes(): Promise<Record<string, string>> {
  const pages: Record<string, string> = {}
  for (const lang of HOME_LANGS) {
    pages[lang.contentLang] = await renderAdtyHome(lang.contentLang)
  }
  return pages
}
