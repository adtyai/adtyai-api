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
import { createContext, useContext, useEffect, useId } from 'react'

export const DraftNavigationContext = createContext<
  ((id: string, dirty: boolean) => void) | null
>(null)

/** Register a panel's unsaved edits with its page's existing navigation guard. */
export function useDraftNavigation(dirty: boolean) {
  const register = useContext(DraftNavigationContext)
  const id = useId()
  useEffect(() => {
    if (!register) return
    register(id, dirty)
    return () => register(id, false)
  }, [register, id, dirty])
}
