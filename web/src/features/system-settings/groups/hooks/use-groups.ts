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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { createGroupMeta, listGroupMetas, putGroupMeta } from '../api'
import type { GroupMetaCreateInput, GroupMetaPutInput } from '../types'

export const GROUPS_QUERY_KEY = ['admin-groups'] as const

export function useGroupMetas(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: GROUPS_QUERY_KEY,
    queryFn: listGroupMetas,
    staleTime: 15_000,
    enabled: options?.enabled ?? true,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useCreateGroupMeta() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: GroupMetaCreateInput) => createGroupMeta(input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: GROUPS_QUERY_KEY })
        void qc.invalidateQueries({ queryKey: ['status'] })
        void qc.invalidateQueries({ queryKey: ['user-groups'] })
        try {
          window.localStorage.removeItem('status')
        } catch {
          /* Storage may be disabled. */
        }
      }
    },
  })
}

export function usePutGroupMeta() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ code, input }: { code: string; input: GroupMetaPutInput }) =>
      putGroupMeta(code, input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: GROUPS_QUERY_KEY })
        void qc.invalidateQueries({ queryKey: ['status'] })
        void qc.invalidateQueries({ queryKey: ['user-groups'] })
        try {
          window.localStorage.removeItem('status')
        } catch {
          /* Storage may be disabled. */
        }
      }
    },
  })
}
