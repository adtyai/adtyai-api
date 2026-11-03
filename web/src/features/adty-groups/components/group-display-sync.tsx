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
import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'

import { useGroupDisplayStore } from '@/stores/adty-group-display-store'
import { useAuthStore } from '@/stores/auth-store'

import { getGroupDisplay, groupDisplayQueryKey } from '../api'

/** Loads group display names into the store; reloads when the session changes. */
export function GroupDisplaySync() {
  const sid = useAuthStore((state) => state.auth.session?.sid ?? '')
  const setGroups = useGroupDisplayStore((state) => state.setGroups)
  const query = useQuery({
    queryKey: [...groupDisplayQueryKey, sid],
    queryFn: getGroupDisplay,
    staleTime: 60_000,
    meta: { errorToast: false },
  })
  useEffect(() => {
    if (query.data) setGroups(query.data)
  }, [query.data, setGroups])
  return null
}
