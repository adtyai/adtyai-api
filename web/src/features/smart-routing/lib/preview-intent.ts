import { WEIGHT_DIM_KEYS } from '../constants'
import type { ScoreWeights } from '../types'

export function sameWeights(a: ScoreWeights, b?: ScoreWeights): boolean {
  if (!b) return false
  return WEIGHT_DIM_KEYS.every(
    (k) => (Number(a[k]) || 0) === (Number(b[k]) || 0)
  )
}

/**
 * Audit P1-3 / N-P1-3: the preview fingerprint (kind, scheme_id, scheme_name,
 * weights) must equal what PUT /config derives server-side in
 * RequirePreviewForConfigChange, otherwise PUT fails with "preview_token does
 * not match". PUT never carries scheme_id, so preview must not either.
 */
export function previewIntentFor(opts: {
  activeScheme?: string
  currentWeights?: ScoreWeights
  nextWeights: ScoreWeights
  schemeName?: string
}): { kind: 'update_weights' | 'activate_scheme' | 'put_config'; scheme_name?: string } {
  const active = opts.activeScheme || undefined
  const schemeChanging = Boolean(opts.schemeName && opts.schemeName !== active)
  const weightsChanging = !sameWeights(opts.nextWeights, opts.currentWeights)
  let kind: 'update_weights' | 'activate_scheme' | 'put_config' =
    'update_weights'
  if (schemeChanging) {
    kind = weightsChanging ? 'put_config' : 'activate_scheme'
  }
  return { kind, scheme_name: schemeChanging ? opts.schemeName : active }
}

/** N-P2-4 (FE guard): weights must be finite, non-negative and sum > 0. */
export function validWeights(w: ScoreWeights): boolean {
  let sum = 0
  for (const k of WEIGHT_DIM_KEYS) {
    const v = Number(w[k])
    if (!Number.isFinite(v) || v < 0) {
      return false
    }
    sum += v
  }
  return sum > 0
}
