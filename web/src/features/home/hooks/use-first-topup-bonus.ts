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

import { getAdtyTopupTiers } from '@/features/wallet/api'
import { firstTopupBonusRateToPercent } from '@/features/wallet/lib/adty-topup'

/**
 * adtyai：读取公开接口 /api/topup/tiers 的首充赠送比例（百分比）。
 * 接口失败、未配置或比例为 0 时返回 null，调用方应不写具体数字。
 */
export function useFirstTopupBonusPercent(): number | null {
  const { data } = useQuery({
    queryKey: ['adty', 'topup-tiers', 'public'],
    queryFn: getAdtyTopupTiers,
    staleTime: 5 * 60 * 1000,
    retry: false,
  })
  const rate = data?.success ? data.data?.first_topup_bonus_rate : undefined
  if (typeof rate !== 'number' || !Number.isFinite(rate) || rate <= 0) {
    return null
  }
  return firstTopupBonusRateToPercent(rate)
}
