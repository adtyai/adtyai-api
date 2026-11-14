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
import { normalizeInterfaceLanguage } from '@/i18n/languages'

/**
 * Locale tags used by the official tawk.to v4 widget language packs
 * (`https://embed.tawk.to/_s/v4/app/<build>/languages/{{lng}}.json`).
 * Simplified Chinese is `zh_cn`, not `zh-CN`. English is `en`.
 * Unofficial packs such as `zh_tw` exist on Tawk's CDN but are not
 * official ADTY product languages and must not be selected.
 */
export const TAWK_LOCALE_EN = 'en'
export const TAWK_LOCALE_ZH_CN = 'zh_cn'

export type OfficialTawkLocale =
  | typeof TAWK_LOCALE_EN
  | typeof TAWK_LOCALE_ZH_CN

const TAWK_WIDGET_SETTINGS_URL = 'va.tawk.to/v1/widget-settings'

export function officialTawkLocale(
  language?: string | null
): OfficialTawkLocale {
  return normalizeInterfaceLanguage(language) === 'zhCN'
    ? TAWK_LOCALE_ZH_CN
    : TAWK_LOCALE_EN
}

export function isTawkWidgetSettingsUrl(value: unknown): boolean {
  const url = tawkRequestUrl(value)
  return url.includes(TAWK_WIDGET_SETTINGS_URL)
}

export function rewriteTawkWidgetSettings(
  payload: unknown,
  locale: OfficialTawkLocale
): unknown {
  if (!payload || typeof payload !== 'object') return payload
  const root = payload as { data?: { widget?: Record<string, unknown> } }
  if (!root.data || typeof root.data !== 'object') return payload
  const widget = root.data.widget
  if (!widget || typeof widget !== 'object') return payload
  return {
    ...root,
    data: {
      ...root.data,
      widget: { ...widget, language: locale },
    },
  }
}

/**
 * Tawk v4 applies the widget language at load from
 * `GET https://va.tawk.to/v1/widget-settings` via
 * `changeLanguage(data.widget.language || "en")`. There is no documented
 * JS API to set language on a single property/widget pair. This hook
 * rewrites that official field to the current official product language
 * so the same site-setting IDs follow zh-CN / en.
 */
export function installTawkSettingsLocaleHook(
  getLocale: () => OfficialTawkLocale
): () => void {
  const restoreFetch = hookTawkSettingsFetch(getLocale)
  const restoreXhr = hookTawkSettingsXhr(getLocale)
  return () => {
    restoreFetch()
    restoreXhr()
  }
}

function hookTawkSettingsFetch(getLocale: () => OfficialTawkLocale): () => void {
  const previous = window.fetch
  if (typeof previous !== 'function') return () => {}
  window.fetch = async (input, init) => {
    const response = await previous.call(window, input, init)
    if (!isTawkWidgetSettingsUrl(input) || !response.ok) {
      return response
    }
    try {
      const rewritten = rewriteTawkWidgetSettings(
        await response.clone().json(),
        getLocale()
      )
      return new Response(JSON.stringify(rewritten), {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
      })
    } catch {
      return response
    }
  }
  return () => {
    window.fetch = previous
  }
}

function hookTawkSettingsXhr(getLocale: () => OfficialTawkLocale): () => void {
  const proto = XMLHttpRequest.prototype
  const previousOpen = proto.open
  const previousSend = proto.send
  proto.open = function (
    this: XMLHttpRequest,
    method: string,
    url: string | URL,
    async?: boolean,
    username?: string | null,
    password?: string | null
  ) {
    ;(this as XMLHttpRequest & { __adtyTawkSettings?: boolean }).__adtyTawkSettings =
      isTawkWidgetSettingsUrl(url)
    return previousOpen.call(
      this,
      method,
      url,
      async ?? true,
      username,
      password
    )
  }
  proto.send = function (this: XMLHttpRequest, body?: Document | XMLHttpRequestBodyInit | null) {
    if ((this as XMLHttpRequest & { __adtyTawkSettings?: boolean }).__adtyTawkSettings) {
      this.addEventListener(
        'readystatechange',
        function () {
          if (this.readyState !== 4 || this.status < 200 || this.status >= 300) {
            return
          }
          try {
            const rewritten = rewriteTawkWidgetSettings(
              JSON.parse(this.responseText),
              getLocale()
            )
            const text = JSON.stringify(rewritten)
            Object.defineProperty(this, 'responseText', {
              configurable: true,
              get: () => text,
            })
            Object.defineProperty(this, 'response', {
              configurable: true,
              get: () => text,
            })
          } catch {
            // Leave the original body if it is not JSON settings.
          }
        },
        true
      )
    }
    return previousSend.call(this, body)
  }
  return () => {
    proto.open = previousOpen
    proto.send = previousSend
  }
}

function tawkRequestUrl(value: unknown): string {
  if (typeof value === 'string') return value
  if (value instanceof URL) return value.href
  if (value && typeof value === 'object' && 'url' in value) {
    return String((value as Request).url)
  }
  return ''
}
