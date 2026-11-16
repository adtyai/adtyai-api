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
import type { AdtyMethod, AdtyOrderStatus } from '../types'

/** Display names; the method IDs are what the API stores. */
export const METHOD_LABELS: Record<string, string> = {
  'usdt.trc20': 'USDT · TRC20',
  'usdt.bep20': 'USDT · BEP20',
  'usdt.polygon': 'USDT · Polygon',
  'usdt.xlayer': 'USDT · X Layer',
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
}

export const CHAIN_NAMES: Record<string, string> = {
  'usdt.trc20': 'TRC20',
  'usdt.bep20': 'BEP20',
  'usdt.polygon': 'Polygon',
  'usdt.xlayer': 'X Layer',
}

export function methodLabel(method: string): string {
  return METHOD_LABELS[method] ?? method
}

/** Epay lists one button per channel, so a method id alone is not unique. */
export function paymentMethodKey(method: AdtyMethod): string {
  return `${method.id}:${method.channel_id ?? 0}`
}

/** Repeated names get a number (Alipay 1, Alipay 2) so every button reads differently. */
export function paymentMethodLabels(labels: string[]): string[] {
  const total = new Map<string, number>()
  for (const label of labels) total.set(label, (total.get(label) ?? 0) + 1)
  const seen = new Map<string, number>()
  return labels.map((label) => {
    if ((total.get(label) ?? 0) < 2) return label
    const next = (seen.get(label) ?? 0) + 1
    seen.set(label, next)
    return `${label} ${next}`
  })
}

export const ORDER_STATUS_LABELS: Record<AdtyOrderStatus, string> = {
  pending: 'Awaiting payment',
  success: 'Credited',
  expired: 'Expired',
  canceled: 'Canceled',
  failed: 'Failed',
  refunded: 'Refunded',
}

export const ORDER_STATUS_VARIANTS: Record<
  AdtyOrderStatus,
  'success' | 'warning' | 'danger' | 'neutral' | 'info'
> = {
  pending: 'warning',
  success: 'success',
  expired: 'neutral',
  canceled: 'neutral',
  failed: 'danger',
  refunded: 'info',
}
