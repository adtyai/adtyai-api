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
import type { SenderDnsRecord, SitePrice } from '../types'

/** True for a decimal string below zero ("-0.00" counts as zero). */
export function isNegativeAmount(value: string | undefined): boolean {
  return Number(value ?? 0) < 0
}

/** A raise that is saved but not in effect yet. */
export type PendingIncrease = {
  group: string
  current: string
  next: string
  at: number
}

/** Pending increases among the site-wide prices, default first then by group. */
export function pendingIncreases(
  prices: SitePrice[] | null | undefined
): PendingIncrease[] {
  return (prices ?? [])
    .filter(
      (price) =>
        price.user_id === 0 &&
        Number(price.pending_markup) > 0 &&
        price.pending_at > 0
    )
    .map((price) => ({
      group: price.group,
      current: price.markup,
      next: price.pending_markup,
      at: price.pending_at,
    }))
    .sort((a, b) => {
      if (a.group === b.group) return 0
      if (a.group === '') return -1
      if (b.group === '') return 1
      return a.group.localeCompare(b.group)
    })
}

/** The site-wide default markup and per-group markups as editable strings. */
export function siteMarkups(prices: SitePrice[] | null | undefined): {
  defaultMarkup: string
  groups: Record<string, string>
} {
  let defaultMarkup = ''
  const groups: Record<string, string> = {}
  for (const price of prices ?? []) {
    if (price.user_id !== 0) continue
    if (price.group === '') defaultMarkup = price.markup
    else groups[price.group] = price.markup
  }
  return { defaultMarkup, groups }
}

/** A fresh idempotency key for one trial grant. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

let rowSeq = 0

/** A key for an editable list row; stays the same while the row is edited. */
export function newRowKey(): string {
  rowSeq += 1
  return `row-${rowSeq}`
}

/** Parses the DNS records stored with a sender domain; bad or empty JSON gives no rows. */
export function parseSenderRecords(raw: string | undefined): SenderDnsRecord[] {
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as SenderDnsRecord[]) : []
  } catch {
    return []
  }
}
