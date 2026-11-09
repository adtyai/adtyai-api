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

const money = z
  .string()
  .regex(/^\d+(\.\d{1,8})?$/, 'Invalid parameters')
  .max(32)
export const paymentSceneSchema = z
  .object({
    key: z.string().regex(/^[a-z][a-z0-9_.-]{0,63}$/, 'Invalid parameters'),
    name: z.string().trim().min(1, 'Required').max(120),
    business: z.literal('wallet_topup'),
    enabled: z.boolean(),
    providers: z.array(z.string()).min(1, 'Required').max(6),
    methods: z.array(z.string().min(1).max(255)).max(100),
    channel_ids: z.array(z.number().int().positive()).max(1000),
    currencies: z
      .array(z.string().regex(/^[A-Z][A-Z0-9]{1,15}$/, 'Invalid parameters'))
      .max(32),
    site_scope: z.enum(['all', 'main', 'all_sites', 'selected']),
    site_ids: z.array(z.number().int().positive()).max(1000),
    user_types: z.array(z.string()).max(6),
    min_usd: money,
    max_usd: money,
  })
  .superRefine((scene, context) => {
    if (
      Number(scene.max_usd) > 0 &&
      Number(scene.min_usd) > Number(scene.max_usd)
    ) {
      context.addIssue({
        code: 'custom',
        path: ['max_usd'],
        message: 'Invalid parameters',
      })
    }
    if (scene.site_scope === 'selected' && scene.site_ids.length === 0) {
      context.addIssue({
        code: 'custom',
        path: ['site_ids'],
        message: 'Required',
      })
    }
  })

export const sceneAudienceLabels = {
  all: 'All sites and main site',
  main: 'Main site',
  all_sites: 'All sub-sites',
  selected: 'Selected sites',
} as const
export const sceneUserLabels = {
  main_customer: 'Main-site customers',
  site_customer: 'Site customers',
  site_owner: 'Site owners',
  affiliate: 'Affiliates',
  admin: 'Administrators',
  root: 'Root',
} as const
