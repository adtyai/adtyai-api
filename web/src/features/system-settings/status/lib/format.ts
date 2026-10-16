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

import dayjs from '@/lib/dayjs'

/** Probe row status for badges and overall health. */
export type ProbeStatus = 'ok' | 'fail' | 'skipped'

export type ProbeLike = {
  ok: boolean
  status?: string | null
  error?: string | null
  target_type?: string
  target_id?: string
}

/**
 * Backend marks empty/unconfigured probes as ok=false with error
 * starting with "skipped:" (e.g. "skipped: no probe url").
 */
export function isProbeSkipped(error: string | null | undefined): boolean {
  if (!error) return false
  return error.trim().toLowerCase().startsWith('skipped')
}

/**
 * Derive display status. Prefer backend `status` (A44); fall back to
 * ok + "skipped" error prefix only when status is missing (older backend).
 */
export function getProbeStatus(probe: ProbeLike): ProbeStatus {
  const s = probe.status?.trim().toLowerCase()
  if (s === 'ok' || s === 'fail' || s === 'skipped') return s
  if (isProbeSkipped(probe.error)) return 'skipped'
  return probe.ok ? 'ok' : 'fail'
}

/** i18n keys for the status badge. */
export function formatProbeStatusLabel(
  status: ProbeStatus
): 'OK' | 'FAIL' | 'Not configured' {
  if (status === 'ok') return 'OK'
  if (status === 'skipped') return 'Not configured'
  return 'FAIL'
}

/** @deprecated Prefer getProbeStatus + formatProbeStatusLabel. */
export function formatOkLabel(ok: boolean): 'OK' | 'FAIL' {
  return ok ? 'OK' : 'FAIL'
}

/** Latency display, e.g. "12 ms" or "—" when negative/unknown. */
export function formatLatencyMs(latencyMs: number | null | undefined): string {
  if (latencyMs == null || Number.isNaN(latencyMs) || latencyMs < 0) {
    return '—'
  }
  return `${Math.round(latencyMs)} ms`
}

/**
 * Format an ISO / RFC3339 timestamp from the API for the table.
 * Empty or invalid values become "—".
 */
export function formatProbeTime(
  value: string | null | undefined
): string {
  if (!value) return '—'
  const d = dayjs(value)
  if (!d.isValid()) return '—'
  return d.format('YYYY-MM-DD HH:mm:ss')
}

/** Stable sort: target_type, then target_id. */
export function sortProbesByTarget<
  T extends { target_type: string; target_id: string },
>(probes: T[]): T[] {
  return [...probes].sort((a, b) => {
    const t = a.target_type.localeCompare(b.target_type)
    if (t !== 0) return t
    return a.target_id.localeCompare(b.target_id)
  })
}

/**
 * Canonical probe name `type/id` (matches accept script keys like epay/1).
 * Unknown types still return the raw `type/id` so new probes render.
 */
export function formatProbeName(targetType: string, targetId: string): string {
  const tt = (targetType || '').trim() || 'unknown'
  const tid = (targetId || '').trim() || 'main'
  return `${tt}/${tid}`
}

export type ProbeDisplay =
  | { kind: 'static'; key: string }
  | { kind: 'withId'; key: string; id: string }
  | { kind: 'raw'; name: string }

/**
 * Resolve a display descriptor for the probe.
 * Known types → i18n key (static or with {{id}}); unknown → raw `type/id`.
 */
export function resolveProbeDisplay(
  targetType: string,
  targetId: string
): ProbeDisplay {
  const tt = (targetType || '').trim().toLowerCase()
  const tid = (targetId || '').trim() || 'main'
  switch (tt) {
    case 'postgres':
      return { kind: 'static', key: 'PostgreSQL' }
    case 'redis':
      return { kind: 'static', key: 'Redis' }
    case 'epay':
      return { kind: 'withId', key: 'Epay #{{id}}', id: tid }
    case 'bepusdt':
      return { kind: 'static', key: 'BEpusdt' }
    case 'chatwoot':
      return { kind: 'static', key: 'Chatwoot' }
    case 'channel':
      return { kind: 'withId', key: 'Channel #{{id}}', id: tid }
    case 'callback':
      return { kind: 'withId', key: 'Callback #{{id}}', id: tid }
    default:
      return { kind: 'raw', name: formatProbeName(targetType, targetId) }
  }
}

/** English fallback title (also used in unit tests). */
export function formatProbeDisplayName(
  targetType: string,
  targetId: string
): string {
  const d = resolveProbeDisplay(targetType, targetId)
  if (d.kind === 'raw') return d.name
  if (d.kind === 'withId') return d.key.replace('{{id}}', d.id)
  return d.key
}

export type ProbeHealthSummary = {
  total: number
  ok: number
  fail: number
  skipped: number
  /** fail count only — skipped is not unhealthy. */
  unhealthy: number
  /** true when there is at least one fail (skipped ignored). */
  hasUnhealthy: boolean
  /** 'empty' | 'healthy' | 'unhealthy' */
  overall: 'empty' | 'healthy' | 'unhealthy'
}

/** Aggregate probe rows; skipped does not count toward unhealthy / overall alert. */
export function summarizeProbeHealth(
  probes: ProbeLike[]
): ProbeHealthSummary {
  let ok = 0
  let fail = 0
  let skipped = 0
  for (const p of probes) {
    const s = getProbeStatus(p)
    if (s === 'ok') ok += 1
    else if (s === 'skipped') skipped += 1
    else fail += 1
  }
  const total = probes.length
  const unhealthy = fail
  const hasUnhealthy = unhealthy > 0
  let overall: ProbeHealthSummary['overall'] = 'empty'
  if (total > 0) {
    overall = hasUnhealthy ? 'unhealthy' : 'healthy'
  }
  return { total, ok, fail, skipped, unhealthy, hasUnhealthy, overall }
}

/**
 * Probe error codes from model.sanitizeProbeErr → i18n key. Unknown / free
 * text (e.g. "http 503") → null so callers render it unchanged.
 */
const probeErrorLabelKeys: Readonly<Record<string, string>> = {
  timeout: 'Timed out',
  'connection refused': 'Connection refused',
  'dns error': 'DNS resolution failed',
  'tls error': 'TLS error',
  'invalid url': 'Invalid URL',
  'request failed': 'Request failed',
}

export function probeErrorLabelKey(error: string): string | null {
  return Object.hasOwn(probeErrorLabelKeys, error)
    ? probeErrorLabelKeys[error]
    : null
}
