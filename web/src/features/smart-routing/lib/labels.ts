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

import { HALF_OPEN_INFLIGHT_MAX, HALF_OPEN_SUCCESS_TARGET } from '../constants'
import type { HalfOpenStage } from '../types'

/**
 * Half-open stage badge text per plan §5.1.
 * Returns an i18n key or interpolated English source string.
 */
export function formatHalfOpenLabel(opts: {
  stage?: HalfOpenStage | string | null
  successCount?: number | null
  inflight?: number | null
  cooldownRemainSec?: number | null
}): string {
  const stage = opts.stage ?? 'unknown'
  if (stage === 'open') {
    const sec = opts.cooldownRemainSec ?? 0
    if (sec > 0) {
      const m = Math.floor(sec / 60)
      const s = sec % 60
      // Pre-baked key for common display; HalfOpenBadge also passes through t()
      return `Cooldown ${m}m ${s}s`
    }
    return 'Circuit open'
  }
  if (stage === 'probe_2') {
    const a = Math.min(
      Math.max(0, Math.floor(opts.successCount ?? 0)),
      HALF_OPEN_SUCCESS_TARGET
    )
    return `Probe 2% · success ${a}/${HALF_OPEN_SUCCESS_TARGET}`
  }
  if (stage === 'ramp_10') {
    return 'Ramp 10%'
  }
  if (stage === 'full') {
    return 'Full'
  }
  if (stage === 'closed') {
    return 'Normal'
  }
  return 'Unknown'
}

export function formatInflightHint(inflight?: number | null): string {
  const a = inflight ?? 0
  return `${a}/${HALF_OPEN_INFLIGHT_MAX}`
}

export function scoreTone(
  score: number | null | undefined
): 'green' | 'yellow' | 'red' | 'muted' {
  if (score == null || Number.isNaN(score)) return 'muted'
  if (score >= 60) return 'green'
  if (score >= 40) return 'yellow'
  return 'red'
}

export function formatCooldown(sec: number | null | undefined): string {
  if (sec == null || sec <= 0) return ''
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${m}:${String(s).padStart(2, '0')}`
}
