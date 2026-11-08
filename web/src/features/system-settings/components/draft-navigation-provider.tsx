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
import { useRouter } from '@tanstack/react-router'
import { useCallback, useState, type ReactNode } from 'react'

import { DraftNavigationContext } from '../hooks/use-draft-navigation'
import { FormNavigationGuard } from './form-navigation-guard'

/** One leave confirmation covers all draft panels on a page. */
export function DraftNavigationProvider(props: { children: ReactNode }) {
  const router = useRouter({ warn: false })
  const [dirtyPanels, setDirtyPanels] = useState<Set<string>>(() => new Set())
  const register = useCallback((id: string, dirty: boolean) => {
    setDirtyPanels((previous) => {
      if (previous.has(id) === dirty) return previous
      const next = new Set(previous)
      if (dirty) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])
  return (
    <DraftNavigationContext.Provider value={register}>
      {props.children}
      {router && <FormNavigationGuard when={dirtyPanels.size > 0} />}
    </DraftNavigationContext.Provider>
  )
}
