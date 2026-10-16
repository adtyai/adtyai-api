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

import type {
  ChatwootConfigView,
  ChatwootFormValues,
  ChatwootPutInput,
} from '../types'

export const CHATWOOT_FORM_DEFAULTS: ChatwootFormValues = {
  script: '',
  website_token: '',
  base_url: '',
  hmac_secret: '',
}

export function formatHmacFingerprint(fp: string | undefined | null): string {
  if (!fp) return '-'
  return fp
}

export function configToForm(view: ChatwootConfigView): ChatwootFormValues {
  return {
    script: view.script ?? '',
    website_token: view.website_token ?? '',
    base_url: view.base_url ?? '',
    hmac_secret: '',
  }
}

/**
 * Build PUT body with pointer semantics: only include fields that changed.
 * Empty / whitespace-only hmac_secret is omitted (leave blank = do not overwrite).
 * Returns null when there is nothing to send.
 */
export function buildChatwootPut(
  next: ChatwootFormValues,
  initial: ChatwootFormValues
): ChatwootPutInput | null {
  const body: ChatwootPutInput = {}

  if (next.script !== initial.script) {
    body.script = next.script
  }

  const token = next.website_token.trim()
  const initialToken = initial.website_token.trim()
  if (token !== initialToken) {
    body.website_token = token
  }

  const base = next.base_url.trim()
  const initialBase = initial.base_url.trim()
  if (base !== initialBase) {
    body.base_url = base
  }

  const hmac = next.hmac_secret.trim()
  if (hmac) {
    body.hmac_secret = hmac
  }

  return Object.keys(body).length === 0 ? null : body
}

/** True when PUT would change script or set a new HMAC secret (server requires second confirmation). */
export function chatwootPutNeedsConfirm(body: ChatwootPutInput): boolean {
  if (body.script !== undefined) return true
  if (body.hmac_secret !== undefined && body.hmac_secret.trim() !== '') {
    return true
  }
  return false
}
