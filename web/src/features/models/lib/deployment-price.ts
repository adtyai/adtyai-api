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
export function deploymentPriceSummary(data: unknown): string {
  if (!data || typeof data !== 'object') return ''
  const record = data as Record<string, unknown>
  const breakdown = record.price_breakdown
  let total: unknown =
    record.estimated_cost ?? record.total_cost ?? record.total
  if (breakdown && typeof breakdown === 'object' && !Array.isArray(breakdown)) {
    const nested = breakdown as Record<string, unknown>
    total = nested.total_cost ?? nested.totalCost ?? nested.TotalCost ?? total
  }
  if (total === undefined || total === null || total === '') return ''
  const currency = String(record.currency ?? '').trim()
  return currency ? `${String(total)} ${currency}` : String(total)
}
