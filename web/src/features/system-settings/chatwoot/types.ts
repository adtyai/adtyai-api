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

export type ChatwootConfigView = {
  script: string
  website_token: string
  base_url: string
  hmac_secret_set: boolean
  hmac_secret_fingerprint?: string
  backup_contact_type: string
  backup_contact: string
  backup_contact_note: string
  /** R-P2-9: e.g. hmac_secret_missing: ... */
  warnings?: string[]
}

export type ChatwootPutInput = {
  script?: string
  website_token?: string
  base_url?: string
  hmac_secret?: string
  confirm?: boolean
}

export type ChatwootGetResponse = {
  success: boolean
  message?: string
  code?: string
  data?: ChatwootConfigView
}

export type ChatwootPutResponse = {
  success: boolean
  message?: string
  code?: string
  data?: ChatwootConfigView
}

export type ChatwootFormValues = {
  script: string
  website_token: string
  base_url: string
  hmac_secret: string
}
