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

import type {
  MenuVisibilityMap,
  MenuVisibilityPutInput,
  MenuVisibilitySide,
  MenuVisibilityView,
  SideDraft,
} from '../types'
import { knownKeySet, knownKeysFor } from './keys'

/** Missing key → visible (true). */
export function menuKeyVisible(
  map: MenuVisibilityMap | null | undefined,
  key: string
): boolean {
  if (!map || !(key in map)) return true
  return map[key] === true
}

/**
 * Build UI draft for one side: known keys default true, overlay server map,
 * extras from server become customKeys.
 */
export function normalizeSideDraft(
  side: MenuVisibilitySide,
  server: MenuVisibilityMap | null | undefined
): SideDraft {
  const known = knownKeysFor(side)
  const knownSet = knownKeySet(side)
  const src = server ?? {}
  const values: MenuVisibilityMap = {}
  for (const def of known) {
    values[def.key] = menuKeyVisible(src, def.key)
  }
  const customKeys: string[] = []
  for (const key of Object.keys(src).sort()) {
    if (!knownSet.has(key)) {
      customKeys.push(key)
      values[key] = src[key] === true
    }
  }
  return { values, customKeys }
}

export function viewToDrafts(view: MenuVisibilityView): {
  user: SideDraft
  admin: SideDraft
} {
  return {
    user: normalizeSideDraft('user', view.user),
    admin: normalizeSideDraft('admin', view.admin),
  }
}

/** Snapshot of draft values for dirty comparison (sorted keys). */
export function draftSnapshot(draft: SideDraft): string {
  const keys = Object.keys(draft.values).sort()
  const obj: MenuVisibilityMap = {}
  for (const k of keys) {
    obj[k] = draft.values[k] === true
  }
  return JSON.stringify(obj)
}

export function sideIsDirty(current: SideDraft, initial: SideDraft): boolean {
  return draftSnapshot(current) !== draftSnapshot(initial)
}

/**
 * Wholesale map for one side: every known + custom key currently in the draft.
 * Empty object is a valid wholesale clear (all keys default visible server-side).
 */
export function buildSideMap(draft: SideDraft): MenuVisibilityMap {
  const out: MenuVisibilityMap = {}
  for (const key of Object.keys(draft.values).sort()) {
    const trimmed = key.trim()
    if (!trimmed) continue
    out[trimmed] = draft.values[key] === true
  }
  return out
}

/**
 * Build PUT body with wholesale semantics.
 * - dirty side → include full map (may be empty {})
 * - clean side → omit (null/undefined so server leaves it alone)
 * Returns null when neither side is dirty.
 */
export function buildMenuVisibilityPut(
  current: { user: SideDraft; admin: SideDraft },
  initial: { user: SideDraft; admin: SideDraft }
): MenuVisibilityPutInput | null {
  const userDirty = sideIsDirty(current.user, initial.user)
  const adminDirty = sideIsDirty(current.admin, initial.admin)
  if (!userDirty && !adminDirty) return null

  const body: MenuVisibilityPutInput = {}
  if (userDirty) {
    body.user = buildSideMap(current.user)
  }
  if (adminDirty) {
    body.admin = buildSideMap(current.admin)
  }
  return body
}

/** Validate a custom key before adding. */
export function normalizeCustomKey(raw: string): string | null {
  const key = raw.trim()
  if (!key) return null
  if (/\s/.test(key)) return null
  return key
}

export function addCustomKey(
  draft: SideDraft,
  side: MenuVisibilitySide,
  raw: string
): { draft: SideDraft; error?: string } {
  const key = normalizeCustomKey(raw)
  if (!key) {
    return { draft, error: 'empty' }
  }
  if (key in draft.values || knownKeySet(side).has(key)) {
    // Already present as known or custom — just ensure it exists
    if (!(key in draft.values)) {
      return {
        draft: {
          values: { ...draft.values, [key]: true },
          customKeys: draft.customKeys.includes(key)
            ? draft.customKeys
            : [...draft.customKeys, key],
        },
      }
    }
    return { draft, error: 'duplicate' }
  }
  return {
    draft: {
      values: { ...draft.values, [key]: true },
      customKeys: [...draft.customKeys, key],
    },
  }
}
