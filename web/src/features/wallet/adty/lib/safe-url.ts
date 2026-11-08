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
// Adapted from the previous fork's payment-url helper (adty task 5.5).
const APP_SCHEMES = new Set(['alipays:', 'alipay:', 'weixin:'])

/**
 * A payment link from a gateway is only opened when it is http(s) or one of
 * the fixed Alipay / WeChat app schemes, so a gateway cannot send javascript:.
 */
const DANGEROUS_QR_SCHEME = /^(javascript|data|vbscript|file|blob):/i

export function isSafePaymentUrl(value: string | null | undefined): boolean {
  const trimmed = value?.trim() || ''
  if (!trimmed || trimmed.includes('\\')) return false
  try {
    const url = new URL(trimmed)
    return (
      url.protocol === 'http:' ||
      url.protocol === 'https:' ||
      APP_SCHEMES.has(url.protocol)
    )
  } catch {
    return false
  }
}

/** QR payloads may be raw text or a payment scheme. Dangerous schemes are not drawn. */
export function isDisplayablePaymentQr(
  value: string | null | undefined
): boolean {
  const trimmed = value?.trim() || ''
  if (!trimmed || trimmed.includes('\\') || trimmed.startsWith('//')) {
    return false
  }
  return !DANGEROUS_QR_SCHEME.test(trimmed)
}

/** Hosted checkouts use ordinary web links and never app schemes or credentials. */
export function isSafeHostedPaymentUrl(
  value: string | null | undefined
): boolean {
  if (!value || value.includes('\\')) return false
  try {
    const url = new URL(value)
    return (
      (url.protocol === 'https:' || url.protocol === 'http:') &&
      !url.username &&
      !url.password
    )
  } catch {
    return false
  }
}
