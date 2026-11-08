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
import { AuthOperationError } from '@/lib/secure-verification'

type HandoffBrowser = {
  document: Document
  replace: (url: string) => void
}

/** Follow only the callback destination reconstructed by the server's OAuth flow. */
export function handoffOAuthLogin(
  data: unknown,
  provider: string,
  browser: HandoffBrowser = {
    document,
    replace: (url) => window.location.replace(url),
  }
): boolean {
  if (!data || typeof data !== 'object' || !('oauth_handoff_url' in data)) {
    return false
  }
  let destination: URL
  try {
    if (typeof data.oauth_handoff_url !== 'string') {
      throw new Error('Invalid handoff')
    }
    destination = new URL(data.oauth_handoff_url)
    if (
      !['https:', 'http:'].includes(destination.protocol) ||
      destination.username ||
      destination.password ||
      destination.pathname !== `/oauth/${provider}` ||
      destination.hash
    ) {
      throw new Error('Invalid handoff')
    }
  } catch {
    throw new AuthOperationError('OAuth failed')
  }
  let policy = browser.document.querySelector<HTMLMetaElement>(
    'meta[name="referrer"]'
  )
  if (!policy) {
    policy = browser.document.createElement('meta')
    policy.name = 'referrer'
    browser.document.head.append(policy)
  }
  policy.content = 'no-referrer'
  browser.replace(destination.toString())
  return true
}
