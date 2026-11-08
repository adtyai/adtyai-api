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
import type { AdtyOrderStatus } from '../types'

/** Display names; the method IDs are what the API stores. */
export const METHOD_LABELS: Record<string, string> = {
  'usdt.trc20': 'USDT · TRC20',
  'usdt.bep20': 'USDT · BEP20',
  'usdt.polygon': 'USDT · Polygon',
  'usdt.xlayer': 'USDT · X Layer',
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  waffo_pancake: 'Waffo Pancake',
}

export const CHAIN_NAMES: Record<string, string> = {
  'usdt.trc20': 'TRC20',
  'usdt.bep20': 'BEP20',
  'usdt.polygon': 'Polygon',
  'usdt.xlayer': 'X Layer',
}

export function methodLabel(method: string): string {
  if (method.startsWith('creem:')) return 'Creem'
  return METHOD_LABELS[method] ?? method
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
