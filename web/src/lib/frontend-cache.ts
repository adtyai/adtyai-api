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
import { HOME_LANG_STORAGE_KEY } from '@/features/adty-seo/home-langs'
import { PENDING_PAYMENT_STORAGE_KEY } from '@/features/wallet/adty/lib/pending-payment'
import { THEME_STORAGE_KEYS } from '@/lib/theme-storage'

const FRONTEND_CACHE_VERSION = 'default-v1'
const FRONTEND_CACHE_VERSION_KEY = 'adtyai:default:cache-version'
const PRESERVED_LOCAL_STORAGE_KEYS = new Set([
  FRONTEND_CACHE_VERSION_KEY,
  ...Object.values(THEME_STORAGE_KEYS),
  'user',
  'uid',
  'oauth:binding:result',
  // ADTY task 8.9: an open order survives the one-time cache clear that the
  // renamed version key triggers, and any later upstream version bump.
  PENDING_PAYMENT_STORAGE_KEY,
  // ADTY task 9.3: the home language picked on the home page
  HOME_LANG_STORAGE_KEY,
])

export function initializeFrontendCache(): void {
  if (typeof window === 'undefined') return

  try {
    const currentVersion = window.localStorage.getItem(
      FRONTEND_CACHE_VERSION_KEY
    )
    if (currentVersion === FRONTEND_CACHE_VERSION) return

    clearLocalUiCache()
    window.localStorage.setItem(
      FRONTEND_CACHE_VERSION_KEY,
      FRONTEND_CACHE_VERSION
    )
  } catch {
    // Storage can be unavailable in private mode; the app should still boot.
  }
}

function clearLocalUiCache(): void {
  const keysToRemove: string[] = []
  for (let index = 0; index < window.localStorage.length; index += 1) {
    const key = window.localStorage.key(index)
    if (key && !PRESERVED_LOCAL_STORAGE_KEYS.has(key)) {
      keysToRemove.push(key)
    }
  }

  keysToRemove.forEach((key) => window.localStorage.removeItem(key))
}
