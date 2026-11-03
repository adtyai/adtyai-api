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

/** Cookie the backend reads at sign-up (adty task 12.5); validated again on the server. */
export const SIGNUP_SOURCE_COOKIE = 'adty_src'
const MAX_AGE_SECONDS = 30 * 24 * 60 * 60
const UTM_KEYS = ['utm_source', 'utm_medium', 'utm_campaign'] as const

/**
 * Builds the cookie value from the first visit: UTM parameters, landing path
 * and the referring host (other sites only). Returns '' when the cookie
 * already exists, so the first touch is kept for 30 days.
 */
export function buildSignupSource(
  location: Pick<Location, 'pathname' | 'search' | 'hostname'>,
  referrer: string,
  existingCookie: string
): string {
  const present = existingCookie
    .split('; ')
    .some((part) => part.startsWith(`${SIGNUP_SOURCE_COOKIE}=`))
  if (present) return ''
  const params = new URLSearchParams(location.search)
  const out = new URLSearchParams()
  for (const key of UTM_KEYS) {
    const value = params.get(key)?.trim()
    if (value) out.set(key, value.slice(0, 64))
  }
  out.set('landing', location.pathname.slice(0, 255))
  try {
    const host = referrer ? new URL(referrer).hostname : ''
    if (host && host !== location.hostname) out.set('ref_host', host)
  } catch {
    // An unparsable referrer is left out.
  }
  return encodeURIComponent(out.toString())
}

/** Writes adty_src once per visitor (SameSite=Lax, 30 days). */
export function captureSignupSource(doc: Document = document): void {
  const value = buildSignupSource(doc.location, doc.referrer, doc.cookie)
  if (!value) return
  const secure = doc.location.protocol === 'https:' ? '; Secure' : ''
  doc.cookie = `${SIGNUP_SOURCE_COOKIE}=${value}; Max-Age=${MAX_AGE_SECONDS}; Path=/; SameSite=Lax${secure}`
}

/** Root-level hook, mounted next to the ?ref= handling in __root.tsx. */
export function useSignupSourceCapture(): void {
  useEffect(() => {
    captureSignupSource()
  }, [])
}
