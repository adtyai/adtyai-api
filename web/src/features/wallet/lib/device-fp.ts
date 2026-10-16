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

const STORAGE_KEY = 'adty:device-fp'

function generateDeviceFp(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

/**
 * Stable per-browser device fingerprint for first-topup device binding.
 * Persisted in localStorage; regenerated only when missing or unreadable.
 */
export function getAdtyDeviceFp(): string {
  if (typeof window === 'undefined') {
    return generateDeviceFp()
  }
  try {
    const existing = window.localStorage.getItem(STORAGE_KEY)
    if (existing && existing.trim().length > 0) {
      return existing.trim()
    }
    const id = generateDeviceFp()
    window.localStorage.setItem(STORAGE_KEY, id)
    return id
  } catch {
    return generateDeviceFp()
  }
}

/** Test helper: storage key used by getAdtyDeviceFp. */
export const ADTY_DEVICE_FP_STORAGE_KEY = STORAGE_KEY
