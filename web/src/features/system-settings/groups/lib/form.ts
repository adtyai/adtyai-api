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
  GroupMetaCreateInput,
  GroupMetaPutInput,
  GroupMetaView,
  GroupRowDraft,
} from '../types'

/** A2 / A08: fixed English group code. */
export const GROUP_CODE_RE = /^[a-z0-9_-]{2,32}$/

export function isValidGroupCode(raw: string): boolean {
  return GROUP_CODE_RE.test(raw.trim())
}

/** Empty or non-numeric text is rejected. Zero is a real ratio, not a blank. */
export function groupRatioDraftValue(raw: string): number | null {
  const text = raw.trim()
  if (text === '') return null
  const ratio = Number(text)
  if (!Number.isFinite(ratio) || ratio < 0) return null
  return ratio
}

export function viewToDraft(view: GroupMetaView): GroupRowDraft {
  return {
    display_name: view.display_name ?? view.code,
    sort: String(view.sort ?? 0),
  }
}

export function parseSort(raw: string): number | null {
  const trimmed = raw.trim()
  if (trimmed === '') return 0
  if (!/^-?\d+$/.test(trimmed)) return null
  const n = Number(trimmed)
  if (!Number.isFinite(n) || !Number.isInteger(n)) return null
  return n
}

export type BuildPutError =
  | 'display_name_required'
  | 'invalid_sort'
  | 'no_changes'

/**
 * Build PUT body for inline edit.
 * Never includes `code` — group code is immutable server-side.
 */
export function buildGroupPut(
  pathCode: string,
  draft: GroupRowDraft,
  initial: GroupMetaView
): { body: GroupMetaPutInput } | { error: BuildPutError } {
  void pathCode // path carries the code; body must not rename it
  const dn = draft.display_name.trim()
  if (dn === '') {
    return { error: 'display_name_required' }
  }
  const sort = parseSort(draft.sort)
  if (sort === null) {
    return { error: 'invalid_sort' }
  }

  const body: GroupMetaPutInput = {}
  const initialDn = (initial.display_name || initial.code).trim()
  if (dn !== initialDn) {
    body.display_name = dn
  }
  if (sort !== (initial.sort ?? 0)) {
    body.sort = sort
  }
  if (body.display_name === undefined && body.sort === undefined) {
    return { error: 'no_changes' }
  }
  // Explicitly ensure no code field ever lands on the wire.
  if ('code' in body) {
    delete (body as { code?: string }).code
  }
  return { body }
}

export type CreateValidateError =
  | 'invalid_code'
  | 'display_name_required'
  | 'invalid_sort'

export function validateCreateInput(input: {
  code: string
  display_name: string
  sort: string
}):
  | { ok: true; body: GroupMetaCreateInput }
  | { ok: false; error: CreateValidateError } {
  const code = input.code.trim()
  if (code === 'auto' || !isValidGroupCode(code)) {
    return { ok: false, error: 'invalid_code' }
  }
  const dn = input.display_name.trim()
  // Empty display_name is allowed on create (server falls back to code).
  const sort = parseSort(input.sort)
  if (sort === null) {
    return { ok: false, error: 'invalid_sort' }
  }
  const body: GroupMetaCreateInput = { code }
  if (dn !== '') {
    body.display_name = dn
  }
  body.sort = sort
  return { ok: true, body }
}

export type GroupTableDraft = {
  ratio: string
  isPublic: boolean
  displayName: string
}

export type GroupTableSaveResult =
  | {
      ok: true
      changed: boolean
      ratio: Record<string, unknown>
      usable: Record<string, unknown>
      displayNames: Record<string, string>
    }
  | { ok: false; error: 'draft_missing' | 'ratio_required' }

/** Build one billing-table save. Rows come only from existing GroupRatio keys. */
export function buildGroupTableSave(input: {
  ratio: Record<string, unknown>
  usable: Record<string, unknown>
  drafts: Record<string, GroupTableDraft | undefined>
  currentDisplayNames: Record<string, string>
}): GroupTableSaveResult {
  const nextRatio: Record<string, unknown> = { ...input.ratio }
  const nextUsable: Record<string, unknown> = {}
  for (const [code, value] of Object.entries(input.usable)) {
    if (Object.prototype.hasOwnProperty.call(input.ratio, code)) {
      nextUsable[code] = value
    }
  }
  const displayNames: Record<string, string> = {}
  for (const code of Object.keys(input.ratio)) {
    if (code === 'auto') continue
    const draft = input.drafts[code]
    if (!draft) return { ok: false, error: 'draft_missing' }
    const ratio = groupRatioDraftValue(draft.ratio)
    if (ratio === null) return { ok: false, error: 'ratio_required' }
    nextRatio[code] = ratio
    if (draft.isPublic) {
      const current = nextUsable[code]
      nextUsable[code] =
        typeof current === 'string' && current.trim()
          ? current
          : draft.displayName.trim() || code
    } else {
      delete nextUsable[code]
    }
    const nextName = draft.displayName.trim()
    const previous = (input.currentDisplayNames[code] ?? '').trim()
    if (nextName !== '' && nextName !== previous) {
      displayNames[code] = nextName
    }
  }
  return {
    ok: true,
    changed:
      JSON.stringify(nextRatio) !== JSON.stringify(input.ratio) ||
      JSON.stringify(nextUsable) !== JSON.stringify(input.usable) ||
      Object.keys(displayNames).length > 0,
    ratio: nextRatio,
    usable: nextUsable,
    displayNames,
  }
}

/** Map API error code to a stable toast key (i18n English source string). */
export function groupApiErrorToastKey(code: string | undefined): string | null {
  switch (code) {
    case 'invalid_code':
      return 'Invalid group code (use a-z, 0-9, _, -; 2–32 chars)'
    case 'exists':
      return 'That group code already exists'
    case 'code_immutable':
      return 'Group code cannot be changed'
    case 'display_name_required':
      return 'Display name is required'
    case 'forbidden':
    case 'root_required':
      return 'Root permission is required to manage group display names.'
    default:
      return null
  }
}
