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
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { useEffect } from 'react'

import { recordReferralClick } from '../api'

const DEVICE_COOKIE = 'adty_did'

/** Records a referral click; failures (bad code, rate limit) are ignored. */
export async function trackReferral(
  code: string,
  landingPath: string
): Promise<void> {
  const trimmed = code.trim()
  if (!trimmed) return
  try {
    await recordReferralClick(trimmed, landingPath)
  } catch {
    // An invalid or disabled code simply does not set the cookie.
  }
}

/** Writes a random device ID once (one year, SameSite=Lax) for the self-invite check. */
export function ensureDeviceCookie(doc: Document = document): void {
  if (
    doc.cookie.split('; ').some((part) => part.startsWith(`${DEVICE_COOKIE}=`))
  ) {
    return
  }
  const id = crypto.randomUUID().replaceAll('-', '')
  const secure = doc.location?.protocol === 'https:' ? '; Secure' : ''
  doc.cookie = `${DEVICE_COOKIE}=${id}; Max-Age=31536000; Path=/; SameSite=Lax${secure}`
}

/**
 * Root-level referral handling (adty tasks 6.2, 6.5): writes the device cookie
 * once, and on any page with ?ref= records the click and drops the parameter.
 */
export function useReferralCapture(): void {
  const navigate = useNavigate()
  const location = useRouterState({ select: (state) => state.location })
  const search = location.search as Record<string, unknown>
  const ref = typeof search.ref === 'string' ? search.ref : ''

  useEffect(() => {
    ensureDeviceCookie()
  }, [])

  useEffect(() => {
    if (!ref) return
    void trackReferral(ref, location.pathname)
    void navigate({
      to: '.',
      search: (prev: Record<string, unknown>) => {
        const next = { ...prev }
        delete next.ref
        return next
      },
      replace: true,
    })
  }, [ref, location.pathname, navigate])
}
