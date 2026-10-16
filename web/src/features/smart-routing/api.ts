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

import { api } from '@/lib/api'

import {
  DEFAULT_ROUTING_CONFIG,
  DEFAULT_SCORE_WEIGHTS,
  SMART_ROUTING_API_PREFIX,
} from './constants'
import {
  fromBackendWeights,
  resolveSchemePreviewArgs,
  toBackendWeights,
} from './lib/weight-map'
import type {
  ApiEnvelope,
  BreakerEvent,
  BreakerStageRow,
  ChannelPriceRow,
  ChannelScoreRow,
  CostGuardRow,
  CostRankingRow,
  DecisionRow,
  RoutingConfig,
  ScoreWeights,
  SimulateResult,
  WeightHistoryEntry,
  WeightPreview,
  WeightPreviewItem,
  WeightScheme,
} from './types'

const softGetConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

async function softGet<T>(
  path: string,
  fallback: T,
  params?: Record<string, string | number | undefined>
): Promise<T> {
  try {
    const res = await api.get<ApiEnvelope<T>>(
      `${SMART_ROUTING_API_PREFIX}${path}`,
      { ...softGetConfig, params }
    )
    if (res.data?.success && res.data.data !== undefined) {
      return res.data.data
    }
    return fallback
  } catch {
    return fallback
  }
}

async function softMutate<T>(
  method: 'post' | 'put' | 'delete',
  path: string,
  body?: unknown
): Promise<ApiEnvelope<T>> {
  try {
    const url = `${SMART_ROUTING_API_PREFIX}${path}`
    let res
    if (method === 'post') {
      res = await api.post<ApiEnvelope<T>>(url, body, softGetConfig)
    } else if (method === 'put') {
      res = await api.put<ApiEnvelope<T>>(url, body, softGetConfig)
    } else {
      res = await api.delete<ApiEnvelope<T>>(url, softGetConfig)
    }
    return res.data ?? { success: false, message: 'empty response' }
  } catch (err) {
    return envelopeFromError<T>(err)
  }
}

/**
 * F-P2-1: on 4xx/5xx keep the backend's {code,message} instead of axios'
 * generic "Request failed with status code 400".
 */
export function envelopeFromError<T>(err: unknown): ApiEnvelope<T> {
  const data = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') {
    const d = data as { message?: unknown; code?: unknown }
    const code = typeof d.code === 'string' ? d.code : undefined
    const message =
      typeof d.message === 'string' && d.message ? d.message : code
    if (message) {
      return { success: false, message, code }
    }
  }
  const message =
    err instanceof Error ? err.message : 'request failed (backend unavailable)'
  return { success: false, message }
}

export async function getRoutingConfig(): Promise<RoutingConfig> {
  const data = await softGet<Record<string, unknown>>('/config', {
    ...DEFAULT_ROUTING_CONFIG,
    weights: { ...DEFAULT_SCORE_WEIGHTS },
  } as unknown as Record<string, unknown>)
  const scheme =
    (typeof data.active_weight_scheme === 'string' &&
      data.active_weight_scheme) ||
    (typeof data.active_weight_scheme_id === 'string' &&
      data.active_weight_scheme_id) ||
    undefined
  const weights = fromBackendWeights(data.weights)
  return {
    ...DEFAULT_ROUTING_CONFIG,
    ...(data as Partial<RoutingConfig>),
    active_weight_scheme: scheme,
    active_weight_scheme_id: scheme,
    weights: { ...DEFAULT_SCORE_WEIGHTS, ...weights },
  }
}

export type PutRoutingConfigBody = Partial<RoutingConfig> & {
  preview_token?: string
}

/** Convert a FE patch into backend field names/shapes. */
export function toBackendConfigPatch(
  patch: PutRoutingConfigBody
): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(patch)) {
    if (v === undefined) continue
    if (k === 'weights' && v && typeof v === 'object') {
      body.weights = toBackendWeights(v as ScoreWeights)
      continue
    }
    if (k === 'active_weight_scheme_id') {
      // Backend field is active_weight_scheme (name), not _id
      if (patch.active_weight_scheme === undefined) {
        body.active_weight_scheme = v
      }
      continue
    }
    body[k] = v
  }
  return body
}

/**
 * PUT /config — audit P1-3 contract (prep/contract-smart-routing-toggle-config.md):
 * backend merges the body (only present fields change), so send only the patch.
 * Toggle: {enabled}. Weights/scheme changes must include preview_token.
 */
export async function putRoutingConfig(
  patch: PutRoutingConfigBody
): Promise<ApiEnvelope<RoutingConfig>> {
  const body = toBackendConfigPatch(patch)
  if (Object.keys(body).filter((k) => k !== 'preview_token').length === 0) {
    return { success: false, message: 'empty_patch' }
  }
  return softMutate<RoutingConfig>('put', '/config', body)
}

function firstString(...vals: unknown[]): string | undefined {
  for (const v of vals) if (typeof v === 'string') return v
  return undefined
}

function normalizeScheme(raw: unknown): WeightScheme | null {
  if (!raw || typeof raw !== 'object') return null
  const row = raw as Record<string, unknown>
  const idVal = row.id
  const name = typeof row.name === 'string' ? row.name : String(idVal ?? '')
  if (!name && idVal == null) return null
  return {
    id: idVal != null ? String(idVal) : name,
    name: name || String(idVal),
    weights: fromBackendWeights(row.weights),
    note: typeof row.note === 'string' ? row.note : undefined,
    created_at: typeof row.created_at === 'string' ? row.created_at : undefined,
    created_by: row.created_by != null ? String(row.created_by) : undefined,
    is_default: Boolean(row.is_default) || name === 'balanced_default',
  }
}

export async function getWeightSchemes(): Promise<WeightScheme[]> {
  const data = await softGet<unknown[]>('/weight-schemes', [])
  if (!Array.isArray(data) || data.length === 0) {
    return [
      {
        id: 'balanced_default',
        name: 'balanced_default',
        weights: { ...DEFAULT_SCORE_WEIGHTS },
        is_default: true,
      },
    ]
  }
  return data.map(normalizeScheme).filter((s): s is WeightScheme => s != null)
}

export async function createWeightScheme(body: {
  name: string
  weights: RoutingConfig['weights']
  note?: string
}): Promise<ApiEnvelope<WeightScheme>> {
  const res = await softMutate<unknown>('post', '/weight-schemes', {
    name: body.name,
    weights: toBackendWeights(body.weights),
    note: body.note,
  })
  if (!res.success) {
    return { success: false, message: res.message }
  }
  const scheme = normalizeScheme(res.data)
  return {
    success: true,
    message: res.message,
    data: scheme ?? undefined,
  }
}

export async function updateWeightScheme(
  id: string,
  body: Partial<WeightScheme> & { preview_token?: string; activate?: boolean }
): Promise<ApiEnvelope<WeightScheme>> {
  const payload: Record<string, unknown> = {}
  if (body.name !== undefined) payload.name = body.name
  if (body.note !== undefined) payload.note = body.note
  if (body.weights !== undefined) {
    payload.weights = toBackendWeights(body.weights)
  }
  if (body.preview_token) payload.preview_token = body.preview_token
  if (body.activate) payload.activate = true
  const res = await softMutate<unknown>('put', `/weight-schemes/${id}`, payload)
  if (!res.success) {
    return { success: false, message: res.message }
  }
  return {
    success: true,
    message: res.message,
    data: normalizeScheme(res.data) ?? undefined,
  }
}

export async function deleteWeightScheme(
  id: string
): Promise<ApiEnvelope<unknown>> {
  return softMutate('delete', `/weight-schemes/${id}`)
}

type BackendPreviewRow = {
  channel_id?: number
  channel_name?: string
  model?: string
  rw_now?: number
  rw_new?: number
  share_now?: number
  share_new?: number
  score_now?: number
  score_new?: number
}

type BackendPreviewPayload = {
  preview?: BackendPreviewRow[]
  items?: BackendPreviewRow[]
  preview_token?: string
  expires_at?: number
  intent?: Record<string, unknown>
}

function mapPreviewItem(row: BackendPreviewRow): WeightPreviewItem {
  return {
    channel_id: Number(row.channel_id) || 0,
    channel_name: row.channel_name,
    model: row.model,
    share_now: Number(row.rw_now ?? row.share_now ?? 0),
    share_new: Number(row.rw_new ?? row.share_new ?? 0),
    score_now: row.score_now != null ? Number(row.score_now) : undefined,
    score_new: row.score_new != null ? Number(row.score_new) : undefined,
  }
}

function normalizePreview(data: unknown): WeightPreview {
  if (!data || typeof data !== 'object') {
    return { items: [] }
  }
  const raw = data as BackendPreviewPayload
  const rows = raw.preview ?? raw.items ?? []
  return {
    items: Array.isArray(rows) ? rows.map(mapPreviewItem) : [],
    preview_token:
      typeof raw.preview_token === 'string' && raw.preview_token
        ? raw.preview_token
        : undefined,
    expires_at: typeof raw.expires_at === 'number' ? raw.expires_at : undefined,
    intent: raw.intent,
  }
}

export async function previewWeightScheme(body: {
  weights: RoutingConfig['weights']
  scheme_id?: string | number
  scheme_name?: string
  kind?: string
}): Promise<WeightPreview> {
  try {
    const schemeArgs = resolveSchemePreviewArgs({
      schemeId: body.scheme_id,
      schemeName: body.scheme_name,
    })
    const payload: Record<string, unknown> = {
      weights: toBackendWeights(body.weights),
      ...schemeArgs,
    }
    if (body.kind) payload.kind = body.kind
    const res = await softMutate<BackendPreviewPayload>(
      'post',
      '/weight-schemes/preview',
      payload
    )
    if (!res.success || res.data === undefined) {
      // F-P2-2: surface failure explicitly (not "no scores yet").
      return {
        items: [],
        error: res.message || 'preview failed',
        error_code: res.code,
      }
    }
    return normalizePreview(res.data)
  } catch (err) {
    const env = envelopeFromError<unknown>(err)
    return { items: [], error: env.message, error_code: env.code }
  }
}

export async function exportWeightScheme(
  id: string
): Promise<Record<string, unknown> | null> {
  return softGet<Record<string, unknown> | null>(
    `/weight-schemes/${id}/export`,
    null
  )
}

export async function importWeightScheme(
  payload: unknown
): Promise<ApiEnvelope<WeightScheme>> {
  const res = await softMutate<unknown>(
    'post',
    '/weight-schemes/import',
    payload
  )
  if (!res.success) {
    return { success: false, message: res.message }
  }
  return {
    success: true,
    message: res.message,
    data: normalizeScheme(res.data) ?? undefined,
  }
}

function normalizeHistoryEntry(raw: unknown): WeightHistoryEntry | null {
  if (!raw || typeof raw !== 'object') return null
  const row = raw as Record<string, unknown>
  const id = row.id != null ? String(row.id) : ''
  if (!id) return null
  const created = firstString(row.created_at, row.at) ?? ''
  let summary = firstString(row.summary, row.kind)
  if (!summary && typeof row.diff === 'string') {
    summary = row.diff.slice(0, 120)
  }
  return {
    id,
    at: created,
    by: (() => {
      const v = row.by ?? row.operator_id
      return v != null ? String(v) : undefined
    })(),
    summary,
    scheme_name:
      typeof row.scheme_name === 'string' ? row.scheme_name : undefined,
    snapshot:
      typeof row.snapshot === 'string' ||
      (row.snapshot && typeof row.snapshot === 'object')
        ? (row.snapshot as string | Record<string, unknown>)
        : undefined,
  }
}

export async function getWeightHistory(): Promise<WeightHistoryEntry[]> {
  const data = await softGet<unknown[]>('/weight-schemes/history', [])
  if (!Array.isArray(data)) return []
  return data
    .map(normalizeHistoryEntry)
    .filter((e): e is WeightHistoryEntry => e != null)
}

export async function rollbackWeightScheme(body: {
  snapshot_id: string | number
  preview_token?: string
}): Promise<ApiEnvelope<unknown>> {
  const payload: Record<string, unknown> = {
    snapshot_id: Number(body.snapshot_id) || body.snapshot_id,
  }
  if (body.preview_token) payload.preview_token = body.preview_token
  return softMutate('post', '/weight-schemes/rollback', payload)
}

export async function simulateRouting(body: {
  model: string
  group?: string
}): Promise<SimulateResult> {
  try {
    const res = await softMutate<SimulateResult>('post', '/simulate', body)
    return res.data ?? { model: body.model, items: [] }
  } catch {
    return { model: body.model, items: [] }
  }
}

export async function getScores(params?: {
  model?: string
}): Promise<ChannelScoreRow[]> {
  return softGet<ChannelScoreRow[]>('/scores', [], params)
}

export async function getScoreDetail(
  channelId: number
): Promise<ChannelScoreRow | null> {
  return softGet<ChannelScoreRow | null>(`/scores/${channelId}`, null)
}

export async function getRankings(params?: {
  model?: string
}): Promise<ChannelScoreRow[]> {
  return softGet<ChannelScoreRow[]>('/rankings', [], params)
}

export async function getBreakerStages(): Promise<BreakerStageRow[]> {
  return softGet<BreakerStageRow[]>('/breaker/stages', [])
}

export async function getBreakerEvents(): Promise<BreakerEvent[]> {
  return softGet<BreakerEvent[]>('/breaker/events', [])
}

export async function getPrices(): Promise<ChannelPriceRow[]> {
  return softGet<ChannelPriceRow[]>('/prices', [])
}

export async function putPrice(
  body: Partial<ChannelPriceRow> & { channel_id: number }
): Promise<ApiEnvelope<ChannelPriceRow>> {
  return softMutate<ChannelPriceRow>('put', '/prices', body)
}

export async function getCostRankings(): Promise<CostRankingRow[]> {
  return softGet<CostRankingRow[]>('/cost/rankings', [])
}

export async function getCostGuards(): Promise<CostGuardRow[]> {
  return softGet<CostGuardRow[]>('/cost/guards', [])
}

export async function getCostSummary(): Promise<Record<string, unknown>> {
  return softGet<Record<string, unknown>>('/cost/summary', {})
}

export async function getDecisions(params?: {
  request_id?: string
  log_id?: string
  page?: number
}): Promise<DecisionRow[]> {
  if (params?.request_id || params?.log_id) {
    const one = await softGet<DecisionRow | null>(
      '/decision',
      null,
      params as Record<string, string | number | undefined>
    )
    return one ? [one] : []
  }
  return softGet<DecisionRow[]>('/decisions', [], params)
}

export async function clearFakeModel(
  channelId: number
): Promise<ApiEnvelope<unknown>> {
  return softMutate('post', '/tags/fake_model/clear', {
    channel_id: channelId,
  })
}

/** Soft-fail clear of a daily cost guard exclusion (root). */
export async function clearCostGuard(
  channelId: number
): Promise<ApiEnvelope<unknown>> {
  return softMutate('post', '/cost/guards/clear', {
    channel_id: channelId,
  })
}

export async function rollbackToOfficial(): Promise<ApiEnvelope<unknown>> {
  return softMutate('post', '/weights/rollback', { to: 'official' })
}
