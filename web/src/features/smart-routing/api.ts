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
  SimulateResult,
  WeightHistoryEntry,
  WeightPreview,
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
    const message =
      err instanceof Error ? err.message : 'request failed (backend unavailable)'
    return { success: false, message }
  }
}

export async function getRoutingConfig(): Promise<RoutingConfig> {
  const data = await softGet<RoutingConfig>('/config', {
    ...DEFAULT_ROUTING_CONFIG,
    weights: { ...DEFAULT_SCORE_WEIGHTS },
  })
  return {
    ...DEFAULT_ROUTING_CONFIG,
    ...data,
    weights: { ...DEFAULT_SCORE_WEIGHTS, ...data.weights },
  }
}

export async function putRoutingConfig(
  patch: Partial<RoutingConfig>
): Promise<ApiEnvelope<RoutingConfig>> {
  return softMutate<RoutingConfig>('put', '/config', patch)
}

export async function getWeightSchemes(): Promise<WeightScheme[]> {
  return softGet<WeightScheme[]>('/weight-schemes', [
    {
      id: 'balanced_default',
      name: 'Balanced',
      weights: { ...DEFAULT_SCORE_WEIGHTS },
      is_default: true,
    },
  ])
}

export async function createWeightScheme(body: {
  name: string
  weights: RoutingConfig['weights']
  note?: string
}): Promise<ApiEnvelope<WeightScheme>> {
  return softMutate<WeightScheme>('post', '/weight-schemes', body)
}

export async function updateWeightScheme(
  id: string,
  body: Partial<WeightScheme>
): Promise<ApiEnvelope<WeightScheme>> {
  return softMutate<WeightScheme>('put', `/weight-schemes/${id}`, body)
}

export async function deleteWeightScheme(
  id: string
): Promise<ApiEnvelope<unknown>> {
  return softMutate('delete', `/weight-schemes/${id}`)
}

export async function previewWeightScheme(body: {
  weights: RoutingConfig['weights']
  scheme_id?: string
}): Promise<WeightPreview> {
  try {
    const res = await softMutate<WeightPreview>(
      'post',
      '/weight-schemes/preview',
      body
    )
    return res.data ?? { items: [] }
  } catch {
    return { items: [] }
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
  return softMutate<WeightScheme>('post', '/weight-schemes/import', payload)
}

export async function getWeightHistory(): Promise<WeightHistoryEntry[]> {
  return softGet<WeightHistoryEntry[]>('/weight-schemes/history', [])
}

export async function rollbackWeightScheme(body: {
  snapshot_id: string
}): Promise<ApiEnvelope<unknown>> {
  return softMutate('post', '/weight-schemes/rollback', body)
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

export async function rollbackToOfficial(): Promise<ApiEnvelope<unknown>> {
  return softMutate('post', '/weights/rollback', { to: 'official' })
}
