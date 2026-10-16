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

export type SystemOptionLike = {
  key: string
  value: string
}

export type BepusdtFormValues = {
  BepusdtBaseURL: string
  BepusdtApiToken: string
  BepusdtLegacyNotifyEnabled: boolean
  BepusdtTradeType: string
  BepusdtCurrency: string
}

export type BepusdtTokenMeta = {
  secretSet: boolean
  fingerprint: string
}

export type BepusdtParsed = {
  form: BepusdtFormValues
  token: BepusdtTokenMeta
}

export const BEPUSDT_DEFAULTS: BepusdtFormValues = {
  BepusdtBaseURL: '',
  BepusdtApiToken: '',
  BepusdtLegacyNotifyEnabled: false,
  BepusdtTradeType: 'usdt.trc20',
  BepusdtCurrency: 'USD',
}

function optionMap(
  options: SystemOptionLike[] | undefined
): Map<string, string> {
  const m = new Map<string, string>()
  for (const o of options ?? []) {
    m.set(o.key, o.value)
  }
  return m
}

function parseBool(raw: string | undefined, fallback: boolean): boolean {
  if (raw == null || raw === '') return fallback
  return raw === 'true' || raw === '1'
}

/** Parse GET /api/option/ rows into form defaults + token fingerprint meta. */
export function parseBepusdtOptions(
  options: SystemOptionLike[] | undefined
): BepusdtParsed {
  const m = optionMap(options)
  const secretSet = parseBool(m.get('BepusdtApiToken.secret_set'), false)
  const fingerprint = (m.get('BepusdtApiToken.fingerprint') ?? '').trim()

  return {
    form: {
      BepusdtBaseURL: (m.get('BepusdtBaseURL') ?? '').trim(),
      // Token plaintext is never returned; keep input empty.
      BepusdtApiToken: '',
      BepusdtLegacyNotifyEnabled: parseBool(
        m.get('BepusdtLegacyNotifyEnabled'),
        false
      ),
      BepusdtTradeType:
        (m.get('BepusdtTradeType') ?? '').trim() ||
        BEPUSDT_DEFAULTS.BepusdtTradeType,
      BepusdtCurrency:
        (m.get('BepusdtCurrency') ?? '').trim().toUpperCase() ||
        BEPUSDT_DEFAULTS.BepusdtCurrency,
    },
    token: {
      secretSet,
      fingerprint,
    },
  }
}

export function sanitizeBepusdtForm(
  values: BepusdtFormValues
): BepusdtFormValues {
  return {
    BepusdtBaseURL: values.BepusdtBaseURL.trim().replace(/\/+$/, ''),
    BepusdtApiToken: values.BepusdtApiToken.trim(),
    BepusdtLegacyNotifyEnabled: values.BepusdtLegacyNotifyEnabled,
    BepusdtTradeType:
      values.BepusdtTradeType.trim().toLowerCase() ||
      BEPUSDT_DEFAULTS.BepusdtTradeType,
    BepusdtCurrency:
      values.BepusdtCurrency.trim().toUpperCase() ||
      BEPUSDT_DEFAULTS.BepusdtCurrency,
  }
}

export type OptionUpdate = { key: string; value: string | boolean }

/**
 * Build per-key PUTs. Empty token is omitted (leave blank = do not overwrite).
 * Only changed keys are included.
 */
export function buildBepusdtUpdates(
  next: BepusdtFormValues,
  initial: BepusdtFormValues
): OptionUpdate[] {
  const sanitized = sanitizeBepusdtForm(next)
  const base = sanitizeBepusdtForm(initial)
  const updates: OptionUpdate[] = []

  if (sanitized.BepusdtBaseURL !== base.BepusdtBaseURL) {
    updates.push({ key: 'BepusdtBaseURL', value: sanitized.BepusdtBaseURL })
  }

  // Leave blank unless updating — never PUT empty token.
  if (sanitized.BepusdtApiToken) {
    updates.push({ key: 'BepusdtApiToken', value: sanitized.BepusdtApiToken })
  }

  if (
    sanitized.BepusdtLegacyNotifyEnabled !== base.BepusdtLegacyNotifyEnabled
  ) {
    updates.push({
      key: 'BepusdtLegacyNotifyEnabled',
      value: sanitized.BepusdtLegacyNotifyEnabled ? 'true' : 'false',
    })
  }

  if (sanitized.BepusdtTradeType !== base.BepusdtTradeType) {
    updates.push({ key: 'BepusdtTradeType', value: sanitized.BepusdtTradeType })
  }

  if (sanitized.BepusdtCurrency !== base.BepusdtCurrency) {
    updates.push({ key: 'BepusdtCurrency', value: sanitized.BepusdtCurrency })
  }

  return updates
}

export function formatTokenFingerprint(fp: string | undefined | null): string {
  if (!fp) return '-'
  return fp
}
