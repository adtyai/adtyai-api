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
import { z } from 'zod'

export const notificationSchema = z
  .object({
    title: z
      .string()
      .trim()
      .min(1, 'Title is required')
      .max(120, 'Invalid parameters'),
    content: z
      .string()
      .trim()
      .min(1, 'Content is required')
      .max(10000, 'Invalid parameters'),
    kind: z.enum(['platform', 'system', 'maintenance', 'rules']),
    audience: z.enum(['all_users', 'all_sites', 'site']),
    target_site_id: z.number().int().nonnegative(),
    business: z.string().trim().max(200, 'Invalid parameters'),
  })
  .superRefine((value, ctx) => {
    if (value.audience === 'site' && value.target_site_id <= 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['target_site_id'],
        message: 'Select a site',
      })
    }
  })

export type NotificationInput = z.infer<typeof notificationSchema>
export const notificationKindLabels: Record<NotificationInput['kind'], string> =
  {
    platform: 'Platform announcement',
    system: 'System notification',
    maintenance: 'Maintenance notification',
    rules: 'Operating rules update',
  }
export const notificationAudienceLabels: Record<
  NotificationInput['audience'],
  string
> = {
  all_users: 'All users',
  all_sites: 'All sites',
  site: 'Selected site',
}
