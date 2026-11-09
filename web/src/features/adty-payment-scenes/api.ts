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

export type PaymentScene = {
  key: string
  name: string
  business: 'wallet_topup'
  enabled: boolean
  providers: string[]
  methods: string[]
  channel_ids: number[]
  currencies: string[]
  site_scope: 'all' | 'main' | 'all_sites' | 'selected'
  site_ids: number[]
  user_types: string[]
  min_usd: string
  max_usd: string
}

export type PaymentSceneConfig = {
  version: number
  scenes: PaymentScene[]
  provider_options?: { provider: string; label: string }[]
  method_options?: {
    id: string
    label?: string
    currency: string
    available: boolean
  }[]
}

const path = '/api/adty/admin/payment-scenes'
export const paymentScenesKey = ['adty-payment-scenes'] as const

export async function getPaymentScenes(): Promise<PaymentSceneConfig> {
  const response = await api.get<{
    success: boolean
    message?: string
    data: PaymentSceneConfig
  }>(path)
  return requireServerSuccess(response.data).data
}

export async function savePaymentScenes(
  config: PaymentSceneConfig
): Promise<PaymentSceneConfig> {
  const response = await api.put<{
    success: boolean
    message?: string
    data: PaymentSceneConfig
  }>(path, { expected_version: config.version, scenes: config.scenes })
  return requireServerSuccess(response.data).data
}
