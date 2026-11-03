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
import { requireServerSuccess } from '@/lib/server-error-message'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

export type EpayMethodConfig = {
  method: 'alipay' | 'wxpay'
  type_code: string
  min_cny: string
  max_cny: string
  daily_limit_cny: string
  return_type?: string
  enabled: boolean
}

export type EpayChannel = {
  id: number
  name: string
  gateway_url: string
  merchant_id: string
  priority: number
  weight: number
  order_mode: 'api' | 'submit'
  probe_mode: string
  status: 'enabled' | 'maintenance' | 'disabled'
  status_reason: string
  unpaid_streak: number
  unpaid_threshold: number
  has_key: boolean
  today_cny: Record<string, string>
  methods: EpayMethodConfig[]
}

export type EpayChannelInput = Omit<
  EpayChannel,
  'id' | 'status' | 'status_reason' | 'unpaid_streak' | 'has_key' | 'today_cny'
> & { merchant_key: string }

export type EpayTestResult = {
  method: string
  ok: boolean
  return_type: string
  disabled: boolean
  message: string
}

export const epayChannelsQueryKey = ['adty-epay-channels'] as const

export async function getEpayChannels(): Promise<EpayChannel[]> {
  const res = await api.get<ApiResponse<EpayChannel[]>>(
    '/api/adty/admin/epay/channels'
  )
  return requireServerSuccess(res.data).data ?? []
}

export async function saveEpayChannel(
  id: number | null,
  input: EpayChannelInput
) {
  const res = id
    ? await api.put<ApiResponse<null>>(
        `/api/adty/admin/epay/channels/${id}`,
        input
      )
    : await api.post<ApiResponse<{ id: number }>>(
        '/api/adty/admin/epay/channels',
        input
      )
  requireServerSuccess(res.data)
}

export async function deleteEpayChannel(id: number) {
  const res = await api.delete<ApiResponse<null>>(
    `/api/adty/admin/epay/channels/${id}`
  )
  requireServerSuccess(res.data)
}

export async function setEpayChannelStatus(
  id: number,
  status: EpayChannel['status']
) {
  const res = await api.post<ApiResponse<null>>(
    `/api/adty/admin/epay/channels/${id}/status`,
    { status }
  )
  requireServerSuccess(res.data)
}

export async function testEpayChannel(id: number): Promise<EpayTestResult[]> {
  const res = await api.post<ApiResponse<EpayTestResult[]>>(
    `/api/adty/admin/epay/channels/${id}/test`
  )
  return requireServerSuccess(res.data).data ?? []
}
