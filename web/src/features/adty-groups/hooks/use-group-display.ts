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
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { pickContentText, toContentLang } from '@/lib/adty-lang'
import { useGroupDisplayStore } from '@/stores/adty-group-display-store'

import type { GroupDisplayItem } from '../api'

export type GroupDisplay = {
  /** Display name: current language → English → ID. */
  name: (id: string) => string
  /** Description: current language → English → ''. */
  desc: (id: string) => string
  /** "Name · ID" for admin pages; just the ID when the name equals it. */
  adminLabel: (id: string) => string
  groups: GroupDisplayItem[]
}

/** Reads display names loaded by GroupDisplaySync; unknown IDs show as is. */
export function useGroupDisplay(): GroupDisplay {
  const { i18n } = useTranslation()
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  const groups = useGroupDisplayStore((state) => state.groups)
  const byId = useMemo(() => {
    const map = new Map<string, GroupDisplayItem>()
    for (const group of groups) map.set(group.id, group)
    return map
  }, [groups])

  const name = useCallback(
    (id: string) => pickContentText(byId.get(id)?.names, lang) || id,
    [byId, lang]
  )
  const desc = useCallback(
    (id: string) => pickContentText(byId.get(id)?.descs, lang),
    [byId, lang]
  )
  const adminLabel = useCallback(
    (id: string) => {
      const display = name(id)
      return display === id ? id : `${display} · ${id}`
    },
    [name]
  )
  // One stable object per language and group list, so callers can list it in
  // effect dependencies without re-running on every render.
  return useMemo(
    () => ({ name, desc, adminLabel, groups }),
    [name, desc, adminLabel, groups]
  )
}
