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
import type { TFunction } from 'i18next'

/** Public method ids from /api/status. Undefined means the list is not loaded. */
export function paymentMethodLabels(
  ids: string[] | null | undefined,
  t: TFunction
): string[] | null {
  if (ids == null) return null
  const labels: string[] = []
  const seen = new Set<string>()
  for (const id of ids) {
    const label = paymentMethodLabel(id, t)
    if (!label || seen.has(label)) continue
    seen.add(label)
    labels.push(label)
  }
  return labels
}

export function paymentMethodSentence(
  ids: string[] | null | undefined,
  t: TFunction
): string {
  const labels = paymentMethodLabels(ids, t)
  if (labels == null) {
    return t('Top up with the payment methods currently enabled.')
  }
  if (labels.length === 0) {
    return t('No payment methods available. Please contact administrator.')
  }
  return t('Top up with {{methods}}.', { methods: labels.join(', ') })
}

function paymentMethodLabel(id: string, t: TFunction): string | null {
  if (id.startsWith('usdt.')) return 'USDT'
  if (id === 'alipay') return t('Alipay')
  if (id === 'wxpay') return t('WeChat Pay')
  return null
}
