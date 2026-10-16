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
import { isCancelledError } from '@tanstack/react-query'
import { isCancel } from 'axios'
import i18next from 'i18next'

import { settingKeyLabel } from '@/features/system-settings/business-params/lib/groups'

import {
  adtyBackendMessageCodes,
  resolveAdtyErrorKey,
  resolveAdtyHttpStatusKey,
} from './adty-error-codes'

export const safeServerErrorMessage = Symbol('safeServerErrorMessage')

const serverErrorMessageKeys = {
  TELEGRAM_OAUTH_NOT_CONFIGURED:
    'Telegram OAuth is not configured or enabled. Please contact your administrator.',
  TELEGRAM_OAUTH_CONFLICT:
    'The telegram OAuth provider name is reserved. Ask your administrator to rename the conflicting custom provider.',
  TELEGRAM_OAUTH_FAILED: 'Telegram authorization failed. Please try again.',
  TELEGRAM_ACCOUNT_NOT_BOUND:
    'This Telegram account is not linked. Sign in using another method and link it first.',
  TELEGRAM_LEGACY_AUTH_REMOVED:
    'Telegram login has changed. Reload the page and start Telegram OAuth again.',
  AUTH_INTERNAL_ERROR: 'Please try again later.',
  SECURITY_VERIFICATION_FAILED: 'Verification failed. Please try again.',
  PASSKEY_RP_ID_UNAVAILABLE:
    'This Passkey domain is not available on this website. Use its original website or another verification method.',
  PASSKEY_RP_ID_INVALID:
    'Invalid Passkey domain. Enter a domain without a scheme, port, path or wildcard.',
  SECURITY_VERIFICATION_FLOW_REQUIRED:
    'This verification method requires its dedicated verification flow.',
  SECURITY_VERIFICATION_LOCKED:
    'Two-factor authentication is temporarily locked.',
  OAUTH_ACCOUNT_MISMATCH:
    'The OAuth account does not match the account linked to your profile.',
  TWOFA_CODE_INVALID: 'The authenticator code is incorrect.',
  TWOFA_ALREADY_ENABLED: 'Two-factor authentication is already enabled.',
  TWOFA_NOT_ENABLED: 'Two-factor authentication is not enabled.',
  PASSKEY_NOT_FOUND: 'No Passkey is registered.',
  AUTH_FLOW_INVALID: 'Verification flow expired',
  SECURITY_PROOF_REQUIRED: 'Additional verification required',
  SECURITY_PROOF_EXPIRED:
    'Security verification has expired. Please verify again.',
  SECURITY_PROOF_INVALID:
    'Security verification is no longer valid. Please verify again.',
  SECURITY_PROOF_SCOPE_MISMATCH: 'Verification does not match this action.',
  SECURITY_PROOF_CONSUMED:
    'This verification has already been used. Please verify again.',
  SECURITY_PROOF_CONTEXT_MISMATCH:
    "Verification does not match this action's details. Please verify again.",
  SECURITY_CONTEXT_INVALID: 'The action details are invalid.',
  SECURITY_ACTION_FORBIDDEN:
    'You do not have permission to perform this action.',
  SECURITY_PROOF_METHOD_MISMATCH:
    'This verification method is not allowed for this action.',
  SECURITY_METHOD_UNAVAILABLE:
    'This verification method is currently unavailable.',
  TWOFA_SETUP_INVALID:
    'The two-factor setup has expired or changed. Start setup again.',

  AUTH_SESSION_LIMIT:
    'Too many active login sessions. On a device where you are already signed in, open Login sessions and use “Sign out other sessions” to revoke them. If you cannot access a signed-in device, reset your password to sign out all sessions.',
  AUTH_SESSION_ISSUANCE_LIMIT:
    'Too many login sessions were created recently. Please wait for the rolling window to pass, then try again.',
  TELEGRAM_BIND_DISABLED: 'Telegram binding is disabled.',
  TELEGRAM_BIND_INVALID_REQUEST:
    'The Telegram authorization request is invalid or expired.',
  TELEGRAM_BIND_FLOW_INVALID:
    'This Telegram binding request has expired or has already been used.',
  TELEGRAM_BIND_SESSION_INVALID:
    'The login session that started this Telegram binding is no longer valid.',
  TELEGRAM_BIND_ALREADY_BOUND: 'This Telegram account is already bound.',
  TELEGRAM_BIND_USER_DELETED: 'This user account no longer exists.',
  TELEGRAM_BIND_USER_DISABLED: 'This user account is disabled.',
  TELEGRAM_BIND_INTERNAL_ERROR: 'Telegram binding failed. Please try again.',
} as const

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object'
}

/** Walk only error origins, never request configs or arbitrary response data. */
export function getServerErrorSources(
  value: unknown
): Record<string | symbol, unknown>[] {
  const sources: Record<string | symbol, unknown>[] = []
  const pending = [value]
  const seen = new Set<object>()
  for (let index = 0; index < pending.length; index++) {
    const source = pending[index]
    if (!isRecord(source) || seen.has(source)) continue
    seen.add(source)
    sources.push(source)
    if (isRecord(source.response)) pending.push(source.response.data)
    pending.push(source.cause, source.error)
  }
  return sources
}

export function getServerErrorMessageKey(value: unknown): string | null {
  for (const source of getServerErrorSources(value)) {
    if (typeof source.code === 'string') {
      const key =
        serverErrorMessageKeys[
          source.code as keyof typeof serverErrorMessageKeys
        ]
      if (key) return key
    }
    if (source[safeServerErrorMessage]) break
  }
  return null
}

/** Codes whose `{key}` param names a business parameter. */
const BUSINESS_PARAM_KEY_CODES: ReadonlySet<string> = new Set([
  'unknown_key',
  'out_of_range',
  'topup_tiers_count_mismatch',
  'topup_tiers_not_ascending',
  'param_rule_violation',
])

/**
 * unknown_key / out_of_range: show the business parameter's localized name
 * instead of its raw key; unmapped keys keep the raw key.
 */
export function businessParamKeyLabel(
  code: unknown,
  params: Record<string, unknown> | undefined
): { key?: string; other_key?: string; tier?: number } {
  if (typeof code !== 'string' || !BUSINESS_PARAM_KEY_CODES.has(code)) return {}
  const out: { key?: string; other_key?: string; tier?: number } = {}
  const other = params?.other_key
  if (typeof other === 'string' && settingKeyLabel(other) !== other) {
    out.other_key = i18next.t(settingKeyLabel(other))
  }
  // index 是 0 起的档位下标，界面显示「第 index+1 档」
  const index = params?.index
  if (typeof index === 'number' && Number.isInteger(index) && index >= 0) {
    out.tier = index + 1
  }
  const raw = params?.key
  if (typeof raw === 'string' && settingKeyLabel(raw) !== raw) {
    out.key = i18next.t(settingKeyLabel(raw))
  }
  return out
}

/**
 * adtyai admin APIs return a stable `code` plus optional `params`; translate
 * them by code so console toasts never surface the English fallback message.
 */
export function getAdtyServerErrorMessage(value: unknown): string | null {
  for (const source of getServerErrorSources(value)) {
    if (source[safeServerErrorMessage]) break
    const resolved = resolveAdtyErrorKey(source.code, source.params)
    if (resolved) {
      return i18next.t(resolved.key, {
        ...resolved.params,
        ...businessParamKeyLabel(source.code, resolved.params),
        interpolation: { escapeValue: false },
      })
    }
  }
  return null
}

export function getServerErrorStatus(value: unknown): number | undefined {
  for (const source of getServerErrorSources(value)) {
    const status = isRecord(source.response)
      ? source.response.status
      : source.status
    if (typeof status === 'number') return status
  }
  return undefined
}

export function isServerErrorCancelled(value: unknown): boolean {
  return getServerErrorSources(value).some(
    (source) =>
      isCancel(source) ||
      isCancelledError(source) ||
      source.name === 'AbortError'
  )
}

function messageText(value: unknown): string | undefined {
  if (
    typeof value !== 'string' ||
    !value.trim() ||
    value.trim() === 'success'
  ) {
    return undefined
  }
  // A proxy's HTML error document is not an actionable API error message.
  if (/^\s*(?:<!doctype|<html[\s>])/i.test(value)) return undefined
  return value
}

/**
 * The backend sends web console messages as English source text in
 * `message_key` (a key of the locale files) plus `message_params`, and keeps
 * the rendered English text in `message` for other API callers.
 */
export function getServerMessage(value: unknown): string | undefined {
  if (!isRecord(value)) return undefined
  if (typeof value.message_key === 'string' && value.message_key) {
    return i18next.t(
      value.message_key,
      isRecord(value.message_params) ? value.message_params : undefined
    )
  }
  return messageText(value.message)
}

/**
 * Translate text the backend stores or sends as an English source key without
 * params (task fail reasons, built-in group descriptions). The same fields also
 * carry upstream errors and administrator text; those have no locale entry and
 * are shown exactly as written, so interpolation and nesting stay off.
 */
export function translateServerText(
  t: (key: string, options?: Record<string, unknown>) => string,
  text: string
): string {
  return t(text, { skipInterpolation: true })
}

function isSecurityVerificationRequest(config: unknown): boolean {
  if (!isRecord(config)) return false
  if (config.singleUseAuthorization) return true
  const headers = config.headers
  if (!isRecord(headers)) return false
  return typeof headers.has === 'function'
    ? Boolean((headers.has as (name: string) => unknown)('X-Security-Proof'))
    : 'X-Security-Proof' in headers
}

/**
 * No recognized code: 401/403/429 fall back to a localized sentence by HTTP
 * status instead of the backend's English text. A localized `message_key` and
 * codes that intentionally surface the backend message keep precedence.
 *
 * Applies to every page using getServerErrorMessage, upstream ones included:
 * if upstream later adds a 403 carrying neither `message_key` nor `code`, it
 * will also show as 「没有权限」 (likewise 401 / 429).
 */
function getHttpStatusFallbackMessage(
  value: unknown,
  sources: Record<string | symbol, unknown>[]
): string | null {
  const key = resolveAdtyHttpStatusKey(getServerErrorStatus(value))
  if (!key) return null
  for (const source of sources) {
    if (typeof source.message_key === 'string' && source.message_key) {
      return null
    }
    // Security-verification requests present the backend's own rejection.
    if (isSecurityVerificationRequest(source.config)) return null
    if (
      typeof source.code === 'string' &&
      adtyBackendMessageCodes.has(source.code)
    ) {
      return null
    }
  }
  return i18next.t(key)
}

export function getServerErrorMessage(
  value: unknown,
  fallback?: string
): string {
  const sources = getServerErrorSources(value)
  // AuthOperationError deliberately hides internal server details. Never unwrap
  // its cause for presentation, even when a caller wraps it in another Error.
  const safe = sources.find((source) => source[safeServerErrorMessage])
  if (safe) {
    return i18next.t(
      getServerErrorMessageKey(safe) ||
        messageText(safe.message) ||
        fallback ||
        'Something went wrong!'
    )
  }
  const key = getServerErrorMessageKey(value)
  if (key) return i18next.t(key)
  const adtyMessage = getAdtyServerErrorMessage(value)
  if (adtyMessage) return adtyMessage
  const statusMessage = getHttpStatusFallbackMessage(value, sources)
  if (statusMessage) return statusMessage
  for (const source of sources) {
    if (
      source instanceof Error ||
      source.isAxiosError ||
      isRecord(source.response)
    ) {
      continue
    }
    const detail = isRecord(source.error)
      ? messageText(source.error.message)
      : messageText(source.error)
    const message =
      getServerMessage(source) || detail || messageText(source.title)
    if (message) return message
  }
  const status = getServerErrorStatus(value)
  if (status === 304) return i18next.t('Content not modified!')
  if (status === 204) return i18next.t('Content not found.')
  for (const source of sources) {
    const message = messageText(source.message)
    if (message) return message
  }
  return messageText(value) || (fallback ?? i18next.t('Something went wrong!'))
}

/** Preserve the payload/cause so every layer can recognize the same failure. */
export function createServerError(value: unknown, fallback?: string): Error {
  return new Error(getServerErrorMessage(value, fallback ?? ''), {
    cause: value,
  })
}

/** Queries reject failed business responses without changing the raw API contract. */
export function requireServerSuccess<T>(response: T): T {
  if (isRecord(response) && response.success === false) {
    throw createServerError(response)
  }
  return response
}
