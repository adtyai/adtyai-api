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

import { DEFAULT_SCORE_WEIGHTS } from '../constants'
import type { ScoreWeights } from '../types'

/** Backend WeightParts JSON keys. */
export type BackendWeights = {
  conn: number
  price: number
  speed: number
  stab: number
  auth: number
}

const FE_TO_BE = {
  connectivity: 'conn',
  price: 'price',
  speed: 'speed',
  stability: 'stab',
  authenticity: 'auth',
} as const

const BE_TO_FE = {
  conn: 'connectivity',
  price: 'price',
  speed: 'speed',
  stab: 'stability',
  auth: 'authenticity',
} as const

function num(v: unknown, fallback: number): number {
  const n = typeof v === 'number' ? v : Number(v)
  return Number.isFinite(n) ? n : fallback
}

/** UI ScoreWeights → backend {conn,price,speed,stab,auth}. */
export function toBackendWeights(w: ScoreWeights): BackendWeights {
  return {
    conn: num(w.connectivity, DEFAULT_SCORE_WEIGHTS.connectivity),
    price: num(w.price, DEFAULT_SCORE_WEIGHTS.price),
    speed: num(w.speed, DEFAULT_SCORE_WEIGHTS.speed),
    stab: num(w.stability, DEFAULT_SCORE_WEIGHTS.stability),
    auth: num(w.authenticity, DEFAULT_SCORE_WEIGHTS.authenticity),
  }
}

/**
 * Backend weights (object or JSON string) → UI ScoreWeights.
 * Accepts either BE short keys or FE long keys.
 */
export function fromBackendWeights(raw: unknown): ScoreWeights {
  let obj: Record<string, unknown> = {}
  if (typeof raw === 'string') {
    try {
      obj = JSON.parse(raw) as Record<string, unknown>
    } catch {
      return { ...DEFAULT_SCORE_WEIGHTS }
    }
  } else if (raw && typeof raw === 'object') {
    obj = raw as Record<string, unknown>
  } else {
    return { ...DEFAULT_SCORE_WEIGHTS }
  }

  return {
    connectivity: num(
      obj.connectivity ?? obj.conn,
      DEFAULT_SCORE_WEIGHTS.connectivity
    ),
    price: num(obj.price, DEFAULT_SCORE_WEIGHTS.price),
    speed: num(obj.speed, DEFAULT_SCORE_WEIGHTS.speed),
    stability: num(
      obj.stability ?? obj.stab,
      DEFAULT_SCORE_WEIGHTS.stability
    ),
    authenticity: num(
      obj.authenticity ?? obj.auth,
      DEFAULT_SCORE_WEIGHTS.authenticity
    ),
  }
}

/** Resolve preview scheme_id (int64) and/or scheme_name for the backend. */
export function resolveSchemePreviewArgs(opts?: {
  schemeId?: string | number | null
  schemeName?: string | null
}): { scheme_id?: number; scheme_name?: string } {
  const out: { scheme_id?: number; scheme_name?: string } = {}
  const name = opts?.schemeName?.trim()
  if (name) out.scheme_name = name

  if (opts?.schemeId == null || opts.schemeId === '') {
    return out
  }
  const raw = String(opts.schemeId)
  const n = Number(raw)
  if (Number.isFinite(n) && n > 0 && String(Math.trunc(n)) === raw) {
    out.scheme_id = Math.trunc(n)
  } else if (!out.scheme_name) {
    // Non-numeric id (e.g. soft-fail "balanced_default") → scheme_name
    out.scheme_name = raw
  }
  return out
}

export { FE_TO_BE, BE_TO_FE }
