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

import {
  createEpayInstance,
  disableEpayInstance,
  enableEpayInstance,
  listEpayInstances,
  setEpayInstancePrimary,
  updateEpayInstance,
} from '../api'
import type { EpayCreateInput, EpayUpdateInput } from '../types'

export const EPAY_INSTANCES_QUERY_KEY = ['admin-epay-instances'] as const

export function useEpayInstances() {
  return useQuery({
    queryKey: EPAY_INSTANCES_QUERY_KEY,
    queryFn: listEpayInstances,
    staleTime: 15_000,
    retry: (count, err) => {
      if ((err as { status?: number })?.status === 403) return false
      return count < 2
    },
  })
}

export function useCreateEpayInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: EpayCreateInput) => createEpayInstance(input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: EPAY_INSTANCES_QUERY_KEY })
      }
    },
  })
}

export function useUpdateEpayInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: EpayUpdateInput }) =>
      updateEpayInstance(id, input),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: EPAY_INSTANCES_QUERY_KEY })
      }
    },
  })
}

export function useSetEpayPrimary() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => setEpayInstancePrimary(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: EPAY_INSTANCES_QUERY_KEY })
      }
    },
  })
}

export function useDisableEpayInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => disableEpayInstance(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: EPAY_INSTANCES_QUERY_KEY })
      }
    },
  })
}

export function useEnableEpayInstance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => enableEpayInstance(id),
    onSuccess: (data) => {
      if (data.success) {
        void qc.invalidateQueries({ queryKey: EPAY_INSTANCES_QUERY_KEY })
      }
    },
  })
}
