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
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { useAdtySite } from '@/features/adty-site/api'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

import {
  installTawkSettingsLocaleHook,
  officialTawkLocale,
  type OfficialTawkLocale,
} from './tawk-locale'

/** The subset of the tawk.to JavaScript API we call. */
type TawkApi = {
  onLoad?: () => void
  hideWidget?: () => void
  showWidget?: () => void
  shutdown?: () => void
}

declare global {
  interface Window {
    Tawk_API?: TawkApi
    Tawk_LoadStart?: Date
    $_Tawk?: { init?: boolean }
  }
}

const TAWK_SCRIPT_ID = 'adty-tawk-script'

// tawk.to property and widget ids are plain tokens; anything else is a typo or
// an injection attempt and must not reach the script URL.
const TAWK_ID_PATTERN = /^[A-Za-z0-9_-]+$/

function officialEmbedSrc(propertyId: string, widgetId: string): string {
  return `https://embed.tawk.to/${encodeURIComponent(propertyId)}/${encodeURIComponent(widgetId)}`
}

function bindPaymentDialogOnLoad() {
  const tawk: TawkApi = window.Tawk_API ?? {}
  const previousOnLoad = tawk.onLoad
  tawk.onLoad = () => {
    previousOnLoad?.()
    // A payment dialog may have opened before the widget finished loading.
    if (useAdtyUiStore.getState().paymentDialogOpen) {
      window.Tawk_API?.hideWidget?.()
    }
  }
  window.Tawk_API = tawk
}

function teardownTawkEmbed() {
  window.Tawk_API?.shutdown?.()
  document.getElementById(TAWK_SCRIPT_ID)?.remove()
  document
    .querySelectorAll('script[src*="embed.tawk.to/_s/"]')
    .forEach((node) => node.remove())
  if (window.$_Tawk) {
    window.$_Tawk.init = undefined
  }
}

/**
 * Loads the official tawk.to embed once both ids are set in the site settings
 * (one property/widget pair) and hides it while a payment dialog is open.
 *
 * Tawk has no documented JS API to switch language on a single widget. The
 * official v4 embed applies language at load from widget-settings
 * (`changeLanguage(data.widget.language || "en")`) using pack tags `en` and
 * `zh_cn`. The settings hook maps the current official UI language onto those
 * tags, and a language switch remounts this same embed so the tag is applied
 * at load. Everything touching window runs in effects, so the landing page
 * can still be prerendered.
 */
export function TawkWidget() {
  const site = useAdtySite()
  const { i18n } = useTranslation()
  const propertyId = site.data?.tawk_property_id?.trim() ?? ''
  const widgetId = site.data?.tawk_widget_id?.trim() ?? ''
  const tawkLocale = officialTawkLocale(i18n.resolvedLanguage || i18n.language)
  const localeRef = useRef<OfficialTawkLocale>(tawkLocale)
  localeRef.current = tawkLocale

  useEffect(() => installTawkSettingsLocaleHook(() => localeRef.current), [])

  useEffect(() => {
    if (!TAWK_ID_PATTERN.test(propertyId) || !TAWK_ID_PATTERN.test(widgetId)) {
      teardownTawkEmbed()
      return
    }
    const src = officialEmbedSrc(propertyId, widgetId)
    const existing = document.getElementById(TAWK_SCRIPT_ID)
    if (
      existing instanceof HTMLScriptElement &&
      existing.src === src &&
      existing.dataset.tawkLocale === tawkLocale
    ) {
      return
    }
    if (existing) {
      teardownTawkEmbed()
    }

    bindPaymentDialogOnLoad()
    window.Tawk_LoadStart = new Date()

    const script = document.createElement('script')
    script.id = TAWK_SCRIPT_ID
    script.async = true
    script.src = src
    script.charset = 'UTF-8'
    script.setAttribute('crossorigin', '*')
    script.dataset.tawkLocale = tawkLocale
    document.body.appendChild(script)

    return () => {
      teardownTawkEmbed()
    }
  }, [propertyId, widgetId, tawkLocale])

  useEffect(
    () =>
      useAdtyUiStore.subscribe((state, previousState) => {
        if (state.paymentDialogOpen === previousState.paymentDialogOpen) return
        // Before the widget loads these are undefined; onLoad catches up.
        if (state.paymentDialogOpen) {
          window.Tawk_API?.hideWidget?.()
        } else {
          window.Tawk_API?.showWidget?.()
        }
      }),
    []
  )

  return null
}
