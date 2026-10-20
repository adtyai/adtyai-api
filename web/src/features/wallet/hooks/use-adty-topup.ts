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
import { useCallback, useEffect, useState } from 'react'

import { getAdtyTopupTiers } from '../api'
import type { AdtyPayMethod, AdtyTopupTier } from '../types'

export function useAdtyTopup() {
  const [tiers, setTiers] = useState<AdtyTopupTier[]>([])
  const [payMethods, setPayMethods] = useState<AdtyPayMethod[]>([])
  const [firstTopupBonusRate, setFirstTopupBonusRate] = useState(0)
  const [firstEligible, setFirstEligible] = useState(false)
  const [loading, setLoading] = useState(true)
  const [available, setAvailable] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refetch = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const tiersRes = await getAdtyTopupTiers()

      if (!tiersRes?.success || !tiersRes.data?.tiers?.length) {
        setAvailable(false)
        setTiers([])
        setPayMethods([])
        setError(tiersRes?.message || 'tiers unavailable')
        return
      }

      const data = tiersRes.data
      setTiers(data.tiers)
      setPayMethods(Array.isArray(data.pay_methods) ? data.pay_methods : [])
      // 首充加送已停，接口字段不再驱动展示或到账。
      setFirstTopupBonusRate(0)
      setFirstEligible(false)
      setAvailable(true)
    } catch (err) {
      setAvailable(false)
      setError(err instanceof Error ? err.message : 'tiers fetch failed')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    queueMicrotask(() => {
      if (!cancelled) void refetch()
    })
    return () => {
      cancelled = true
    }
  }, [refetch])

  return {
    tiers,
    payMethods,
    firstTopupBonusRate,
    firstEligible,
    loading,
    available,
    error,
    refetch,
  }
}
