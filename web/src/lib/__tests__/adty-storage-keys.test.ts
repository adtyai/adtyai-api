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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PENDING_PAYMENT_STORAGE_KEY } from '@/features/wallet/adty/lib/pending-payment'
import { publishAuthSessionEvent } from '@/lib/auth-session-sync'
import { initializeFrontendCache } from '@/lib/frontend-cache'
import { hasSessionHint, SESSION_HINT_COOKIE_NAME } from '@/lib/session-hint'
import { THEME_STORAGE_KEYS } from '@/lib/theme-storage'

// Task 8.9 renamed the browser-visible cookie and storage keys. The backend
// writes the same cookie name (service/auth_session.go, TestAuthCookieNames).

function clearCookies() {
  for (const cookie of document.cookie.split(';')) {
    const name = cookie.trim().split('=')[0]
    if (name) document.cookie = `${name}=; max-age=0; path=/`
  }
}

beforeEach(() => {
  localStorage.clear()
  clearCookies()
})

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
  clearCookies()
})

describe('session hint cookie', () => {
  it('reads the adtyai_has_session cookie the server writes', () => {
    expect(SESSION_HINT_COOKIE_NAME).toBe('adtyai_has_session')

    document.cookie = 'adtyai_has_session=1; path=/'

    expect(hasSessionHint()).toBe(true)
  })

  it('ignores the upstream cookie name', () => {
    document.cookie = 'new_api_has_session=1; path=/'

    expect(hasSessionHint()).toBe(false)
  })
})

describe('local storage keys', () => {
  it('stores every theme preference under the adtyai prefix', () => {
    expect(Object.values(THEME_STORAGE_KEYS)).toEqual([
      'adtyai:theme:v1:mode',
      'adtyai:theme:v1:preset',
      'adtyai:theme:v1:font',
      'adtyai:theme:v1:radius',
      'adtyai:theme:v1:scale',
      'adtyai:theme:v1:content-layout',
    ])
  })

  it('keeps the pending payment and theme when the cache version changes', () => {
    localStorage.setItem(PENDING_PAYMENT_STORAGE_KEY, '{"tradeNo":"T1"}')
    localStorage.setItem(THEME_STORAGE_KEYS.mode, 'dark')
    localStorage.setItem('stale-ui-cache', 'old')

    initializeFrontendCache()

    expect(localStorage.getItem(PENDING_PAYMENT_STORAGE_KEY)).toBe(
      '{"tradeNo":"T1"}'
    )
    expect(localStorage.getItem(THEME_STORAGE_KEYS.mode)).toBe('dark')
    expect(localStorage.getItem('stale-ui-cache')).toBeNull()
    expect(localStorage.getItem('adtyai:default:cache-version')).toBe(
      'default-v1'
    )
  })

  it('syncs login state between tabs on the adtyai channel', () => {
    const channels: string[] = []
    vi.stubGlobal(
      'BroadcastChannel',
      class {
        constructor(name: string) {
          channels.push(name)
        }
        postMessage() {}
      }
    )

    publishAuthSessionEvent('signed_out', 'sid-1')

    expect(channels).toEqual(['adtyai:auth-session'])
  })
})
