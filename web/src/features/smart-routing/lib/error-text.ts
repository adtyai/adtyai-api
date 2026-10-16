import type { TFunction } from 'i18next'

/** F-P2-1: map backend error codes to readable, translated reasons. */
const CODE_KEYS: Record<string, string> = {
  preview_required:
    'Preview expired or does not match this change; preview again',
  preview_invalid: 'Preview expired or already used; preview again',
  forbidden: 'Only root can change routing strategy',
  invalid_weights: 'Weights must be non-negative and sum to more than 0',
  empty_patch: 'Nothing to save',
  save_failed: 'Save failed on server',
  bad_snapshot: 'This history snapshot cannot be restored',
}

export function routingErrorText(
  t: TFunction,
  res: { message?: string; code?: string } | undefined,
  fallback: string
): string {
  const key = res?.code ? CODE_KEYS[res.code] : undefined
  if (key) {
    return res?.message && res.message !== res.code
      ? `${t(key)} (${res.message})`
      : t(key)
  }
  return res?.message || fallback
}
