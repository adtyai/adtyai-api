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
import { useEffect } from 'react'

import { useAdtySite } from '@/features/adty-site/api'
import { useAdtyUiStore } from '@/stores/adty-ui-store'

/** The subset of the tawk.to JavaScript API we call. */
type TawkApi = {
  onLoad?: () => void
  hideWidget?: () => void
  showWidget?: () => void
}

declare global {
  interface Window {
    Tawk_API?: TawkApi
    Tawk_LoadStart?: Date
  }
}

const TAWK_SCRIPT_ID = 'adty-tawk-script'

// tawk.to property and widget ids are plain tokens; anything else is a typo or
// an injection attempt and must not reach the script URL.
const TAWK_ID_PATTERN = /^[A-Za-z0-9_-]+$/

/**
 * Loads the tawk.to chat widget once both ids are set in the site settings
 * (adty task 8.8) and hides it while a payment dialog is open.
 *
 * The widget language comes from the tawk.to dashboard: tawk.to has no
 * supported runtime call to switch it, so the current i18n language is not
 * applied. Everything touching window runs in effects, so the landing page
 * can still be prerendered.
 */
export function TawkWidget() {
  const site = useAdtySite()
  const propertyId = site.data?.tawk_property_id?.trim() ?? ''
  const widgetId = site.data?.tawk_widget_id?.trim() ?? ''

  useEffect(() => {
    if (!TAWK_ID_PATTERN.test(propertyId) || !TAWK_ID_PATTERN.test(widgetId)) {
      return
    }
    if (document.querySelector(`#${TAWK_SCRIPT_ID}`)) return

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
    window.Tawk_LoadStart = new Date()

    const script = document.createElement('script')
    script.id = TAWK_SCRIPT_ID
    script.async = true
    script.src = `https://embed.tawk.to/${encodeURIComponent(propertyId)}/${encodeURIComponent(widgetId)}`
    script.charset = 'UTF-8'
    script.setAttribute('crossorigin', '*')
    document.body.appendChild(script)
  }, [propertyId, widgetId])

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
