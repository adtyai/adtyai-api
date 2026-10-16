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

import { describe, expect, it } from 'vitest'

import {
  formatLatencyMs,
  formatOkLabel,
  formatProbeDisplayName,
  formatProbeName,
  formatProbeStatusLabel,
  formatProbeTime,
  getProbeStatus,
  isProbeSkipped,
  resolveProbeDisplay,
  sortProbesByTarget,
  summarizeProbeHealth,
} from '../format'

describe('formatOkLabel', () => {
  it('maps true to OK and false to FAIL', () => {
    expect(formatOkLabel(true)).toBe('OK')
    expect(formatOkLabel(false)).toBe('FAIL')
  })
})

describe('isProbeSkipped / getProbeStatus', () => {
  it('detects skipped from error prefix', () => {
    expect(isProbeSkipped('skipped: no probe url')).toBe(true)
    expect(isProbeSkipped('skipped: redis not enabled')).toBe(true)
    expect(isProbeSkipped('  Skipped: empty  ')).toBe(true)
    expect(isProbeSkipped('timeout')).toBe(false)
    expect(isProbeSkipped('')).toBe(false)
    expect(isProbeSkipped(null)).toBe(false)
  })

  it('prefers backend status field over ok/error', () => {
    expect(getProbeStatus({ ok: true, status: 'ok', error: '' })).toBe('ok')
    expect(getProbeStatus({ ok: false, status: 'fail', error: 'x' })).toBe('fail')
    expect(
      getProbeStatus({ ok: false, status: 'skipped', error: 'not configured' })
    ).toBe('skipped')
    // status wins even if error text has a skipped prefix
    expect(
      getProbeStatus({ ok: false, status: 'fail', error: 'skipped: x' })
    ).toBe('fail')
    // unknown/empty status -> legacy fallback
    expect(getProbeStatus({ ok: true, status: '', error: '' })).toBe('ok')
    expect(
      getProbeStatus({ ok: false, status: null, error: 'skipped: x' })
    ).toBe('skipped')
  })

  it('summarizes using status field', () => {
    const r = summarizeProbeHealth([
      { ok: false, status: 'skipped', error: 'none' },
      { ok: true, status: 'ok' },
    ])
    expect(r.skipped).toBe(1)
    expect(r.hasUnhealthy).toBe(false)
  })

  it('maps ok / fail / skipped (legacy, no status)', () => {
    expect(getProbeStatus({ ok: true, error: '' })).toBe('ok')
    expect(getProbeStatus({ ok: false, error: 'timeout' })).toBe('fail')
    expect(
      getProbeStatus({ ok: false, error: 'skipped: no probe url' })
    ).toBe('skipped')
    // skipped wins even if somehow ok=true
    expect(
      getProbeStatus({ ok: true, error: 'skipped: no probe url' })
    ).toBe('skipped')
  })

  it('formats status labels as i18n keys', () => {
    expect(formatProbeStatusLabel('ok')).toBe('OK')
    expect(formatProbeStatusLabel('fail')).toBe('FAIL')
    expect(formatProbeStatusLabel('skipped')).toBe('Not configured')
  })
})

describe('summarizeProbeHealth', () => {
  it('excludes skipped from unhealthy / overall alert', () => {
    const s = summarizeProbeHealth([
      { ok: true, error: '' },
      { ok: false, error: 'skipped: no probe url' },
      { ok: false, error: 'skipped: redis not enabled' },
      { ok: false, error: 'timeout' },
    ])
    expect(s).toEqual({
      total: 4,
      ok: 1,
      fail: 1,
      skipped: 2,
      unhealthy: 1,
      hasUnhealthy: true,
      overall: 'unhealthy',
    })
  })

  it('is healthy when only ok + skipped', () => {
    const s = summarizeProbeHealth([
      { ok: true, error: '' },
      { ok: false, error: 'skipped: no probe url' },
    ])
    expect(s.unhealthy).toBe(0)
    expect(s.hasUnhealthy).toBe(false)
    expect(s.overall).toBe('healthy')
    expect(s.skipped).toBe(1)
  })

  it('is empty when no probes', () => {
    expect(summarizeProbeHealth([])).toEqual({
      total: 0,
      ok: 0,
      fail: 0,
      skipped: 0,
      unhealthy: 0,
      hasUnhealthy: false,
      overall: 'empty',
    })
  })
})

describe('formatProbeName / resolveProbeDisplay', () => {
  it('builds type/id names for new A44 probes', () => {
    expect(formatProbeName('epay', '3')).toBe('epay/3')
    expect(formatProbeName('bepusdt', 'main')).toBe('bepusdt/main')
    expect(formatProbeName('chatwoot', 'main')).toBe('chatwoot/main')
    expect(formatProbeName('postgres', '')).toBe('postgres/main')
  })

  it('resolves known labels and falls back for unknown', () => {
    expect(resolveProbeDisplay('epay', '7')).toEqual({
      kind: 'withId',
      key: 'Epay #{{id}}',
      id: '7',
    })
    expect(resolveProbeDisplay('bepusdt', 'main')).toEqual({
      kind: 'static',
      key: 'BEpusdt',
    })
    expect(resolveProbeDisplay('chatwoot', 'main')).toEqual({
      kind: 'static',
      key: 'Chatwoot',
    })
    expect(resolveProbeDisplay('postgres', 'main')).toEqual({
      kind: 'static',
      key: 'PostgreSQL',
    })
    expect(resolveProbeDisplay('redis', 'main')).toEqual({
      kind: 'static',
      key: 'Redis',
    })
    expect(resolveProbeDisplay('weird', 'x')).toEqual({
      kind: 'raw',
      name: 'weird/x',
    })
    expect(formatProbeDisplayName('epay', '3')).toBe('Epay #3')
    expect(formatProbeDisplayName('mystery', 'z')).toBe('mystery/z')
  })
})

describe('formatLatencyMs', () => {
  it('formats non-negative latency with ms suffix', () => {
    expect(formatLatencyMs(0)).toBe('0 ms')
    expect(formatLatencyMs(12)).toBe('12 ms')
    expect(formatLatencyMs(12.6)).toBe('13 ms')
  })

  it('returns em dash for nullish or negative', () => {
    expect(formatLatencyMs(null)).toBe('—')
    expect(formatLatencyMs(undefined)).toBe('—')
    expect(formatLatencyMs(-1)).toBe('—')
    expect(formatLatencyMs(Number.NaN)).toBe('—')
  })
})

describe('formatProbeTime', () => {
  it('formats ISO timestamps', () => {
    expect(formatProbeTime('2026-10-16T01:02:03Z')).toMatch(
      /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/
    )
  })

  it('returns em dash for empty or invalid', () => {
    expect(formatProbeTime(null)).toBe('—')
    expect(formatProbeTime(undefined)).toBe('—')
    expect(formatProbeTime('')).toBe('—')
    expect(formatProbeTime('not-a-date')).toBe('—')
  })
})

describe('sortProbesByTarget', () => {
  it('sorts by target_type then target_id', () => {
    const rows = [
      { target_type: 'redis', target_id: 'main' },
      { target_type: 'epay', target_id: 'b' },
      { target_type: 'epay', target_id: 'a' },
      { target_type: 'postgres', target_id: 'main' },
    ]
    expect(sortProbesByTarget(rows).map((r) => `${r.target_type}:${r.target_id}`)).toEqual([
      'epay:a',
      'epay:b',
      'postgres:main',
      'redis:main',
    ])
  })
})
