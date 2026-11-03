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
import { safeJsonParse } from '../utils/json-parser'

// One row per group ID. The ID is fixed; names and descriptions live in the
// group table (adty-groups) and are edited in a dialog. The top-up ratio is no
// longer edited here: an absent key means 1 (common/topup-ratio.go).
export type GroupPricingRow = {
  id: string
  ratio: string
  selectable: boolean
  description: string
}

function normalizeRatio(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 1
}

function parseRatioMap(value: string): Record<string, number> {
  return safeJsonParse<Record<string, number>>(value, {
    fallback: {},
    silent: true,
  })
}

function parseUsableMap(value: string): Record<string, string> {
  return safeJsonParse<Record<string, string>>(value, {
    fallback: {},
    silent: true,
  })
}

// "auto" may sit in UserUsableGroups but is not a group (it has no ratio), so
// it never becomes a row; serializeGroupPricingRows carries it through as is.
const AUTO_GROUP_KEY = 'auto'

export function buildGroupPricingRows(
  groupRatio: string,
  userUsableGroups: string,
  extraIds: string[]
): GroupPricingRow[] {
  const ratioMap = parseRatioMap(groupRatio)
  const usableMap = parseUsableMap(userUsableGroups)
  const ids = new Set([
    ...Object.keys(ratioMap),
    ...Object.keys(usableMap),
    ...extraIds,
  ])
  ids.delete(AUTO_GROUP_KEY)

  return [...ids].map((id) => ({
    id,
    ratio: String(normalizeRatio(ratioMap[id])),
    selectable: Object.hasOwn(usableMap, id),
    description: String(usableMap[id] ?? ''),
  }))
}

export function serializeGroupPricingRows(
  rows: GroupPricingRow[],
  sourceUsableGroups: string
) {
  const groupRatio: Record<string, number> = {}
  const userUsableGroups: Record<string, string> = {}
  const sourceUsable = parseUsableMap(sourceUsableGroups)
  if (Object.hasOwn(sourceUsable, AUTO_GROUP_KEY)) {
    userUsableGroups[AUTO_GROUP_KEY] = sourceUsable[AUTO_GROUP_KEY]
  }

  for (const row of rows) {
    groupRatio[row.id] = normalizeRatio(row.ratio)
    if (row.selectable) {
      userUsableGroups[row.id] = row.description
    }
  }

  return {
    GroupRatio: JSON.stringify(groupRatio, null, 2),
    UserUsableGroups: JSON.stringify(userUsableGroups, null, 2),
  }
}

export function groupPricingSignature(
  rows: GroupPricingRow[],
  sourceUsableGroups: string
): string {
  const serialized = serializeGroupPricingRows(rows, sourceUsableGroups)
  return JSON.stringify({
    groupRatio: parseRatioMap(serialized.GroupRatio),
    userUsableGroups: parseUsableMap(serialized.UserUsableGroups),
  })
}
