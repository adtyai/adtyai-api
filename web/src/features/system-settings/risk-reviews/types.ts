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
export type RiskReviewStatus = 'pending' | 'approved' | 'rejected' | string

export type RiskReview = {
  id: number
  kind: string
  user_id: number
  order_no: string
  amount_usd: number
  status: RiskReviewStatus
  reason: string
  payload: string
  resolved_by: number
  created_at: string
  resolved_at?: string | null
}

export type RiskReviewListResponse = {
  success: boolean
  message?: string
  data?: RiskReview[]
  total?: number
}

export type RiskReviewDetailResponse = {
  success: boolean
  message?: string
  data?: RiskReview
}

export type RiskReviewActionResponse = {
  success: boolean
  message?: string
  code?: string
  data?: RiskReview
}
