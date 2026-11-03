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
  CampaignInput,
  CampaignStatus,
  ContentTexts,
  MarketingCampaign,
} from '../api'

/** Template variables an editor can insert; the server maps them to Resend contact properties. */
export const CAMPAIGN_VARIABLES = ['{{username}}', '{{balance}}'] as const

export const CAMPAIGN_STATUS_LABELS: Record<CampaignStatus, string> = {
  draft: 'Draft',
  preparing: 'Preparing',
  scheduled: 'Scheduled',
  sent: 'Sent',
  canceled: 'Canceled',
  failed: 'Failed',
}

export const CAMPAIGN_STATUS_VARIANTS: Record<
  CampaignStatus,
  'neutral' | 'info' | 'success' | 'warning' | 'danger'
> = {
  draft: 'neutral',
  preparing: 'info',
  scheduled: 'info',
  sent: 'success',
  canceled: 'warning',
  failed: 'danger',
}

export function emptyCampaign(includeSites: boolean): CampaignInput {
  return {
    name: '',
    is_template: false,
    subjects: {},
    bodies: {},
    audience: { include_site_customers: includeSites },
    scheduled_at: 0,
  }
}

/** Starts a new draft from a campaign or template. */
export function campaignToInput(campaign: MarketingCampaign): CampaignInput {
  return {
    name: campaign.name,
    is_template: campaign.is_template,
    subjects: { ...campaign.subjects },
    bodies: { ...campaign.bodies },
    audience: { ...campaign.audience },
    scheduled_at: campaign.scheduled_at,
  }
}

/** A language version is complete when both subject and body are filled. */
export function hasVersion(
  subjects: ContentTexts,
  bodies: ContentTexts,
  lang: string
): boolean {
  return Boolean(subjects[lang]?.trim() && bodies[lang]?.trim())
}

/** yyyy-mm-dd in local time → unix seconds at local midnight; '' → 0. */
export function dateInputToUnix(value: string): number {
  if (!value) return 0
  const [year, month, day] = value.split('-').map(Number)
  if (!year || !month || !day) return 0
  return Math.floor(new Date(year, month - 1, day).getTime() / 1000)
}

export function unixToDateInput(value: number | undefined): string {
  if (!value) return ''
  const date = new Date(value * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}
