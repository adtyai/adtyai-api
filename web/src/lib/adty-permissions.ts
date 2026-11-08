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
import type { AuthUser } from '@/stores/auth-store'

import {
  hasPermission,
  type AdminPermissionMatrix,
  type PermissionCatalog,
} from './admin-permissions'
import { ROLE } from './roles'

/** One admin permission: resource + action (mirrors service/authz/resources_adty.go). */
export type AdtyPermission = { resource: string; action: string }

export const ADTY_PERMISSIONS = {
  USER_READ: { resource: 'adty_user', action: 'read' },
  USER_CREATE: { resource: 'adty_user', action: 'create' },
  USER_WRITE: { resource: 'adty_user', action: 'write' },
  USER_GROUP: { resource: 'adty_user', action: 'group' },
  USER_QUOTA: { resource: 'adty_user', action: 'quota' },
  USER_DELETE: { resource: 'adty_user', action: 'delete' },
  LOG_READ: { resource: 'adty_log', action: 'read' },
  LOG_EXPORT: { resource: 'adty_log', action: 'export' },
  MODEL_READ: { resource: 'adty_model', action: 'read' },
  MODEL_CREATE: { resource: 'adty_model', action: 'create' },
  MODEL_WRITE: { resource: 'adty_model', action: 'write' },
  MODEL_DELETE: { resource: 'adty_model', action: 'delete' },
  FINANCE_READ: { resource: 'adty_finance', action: 'read' },
  FINANCE_MANUAL_CREDIT: { resource: 'adty_finance', action: 'manual_credit' },
  FINANCE_REFUND: { resource: 'adty_finance', action: 'refund' },
  REFERRAL_READ: { resource: 'adty_referral', action: 'read' },
  REFERRAL_REVIEW: { resource: 'adty_referral', action: 'review' },
  REFERRAL_PAYOUT: { resource: 'adty_referral', action: 'payout' },
  REFERRAL_ADJUST: { resource: 'adty_referral', action: 'adjust' },
  REFERRAL_SETTINGS_READ: {
    resource: 'adty_referral',
    action: 'settings_read',
  },
  REFERRAL_SETTINGS: { resource: 'adty_referral', action: 'settings' },
  PAYMENT_READ: { resource: 'adty_payment', action: 'read' },
  PAYMENT_OPERATE: { resource: 'adty_payment', action: 'operate' },
  MARKETING_READ: { resource: 'adty_marketing', action: 'read' },
  MARKETING_CREATE: { resource: 'adty_marketing', action: 'create' },
  MARKETING_WRITE: { resource: 'adty_marketing', action: 'write' },
  MARKETING_DELETE: { resource: 'adty_marketing', action: 'delete' },
  MARKETING_SEND: { resource: 'adty_marketing', action: 'send' },
  SITE_READ: { resource: 'adty_site', action: 'read' },
  SITE_CREATE: { resource: 'adty_site', action: 'create' },
  SITE_REVIEW: { resource: 'adty_site', action: 'review' },
  SITE_ADJUST: { resource: 'adty_site', action: 'adjust' },
  SITE_DELETE: { resource: 'adty_site', action: 'delete' },
  SITE_SETTINGS_READ: { resource: 'adty_site', action: 'settings_read' },
  SITE_SETTINGS_CREATE: { resource: 'adty_site', action: 'settings_create' },
  SITE_SETTINGS_DELETE: { resource: 'adty_site', action: 'settings_delete' },
  SITE_SETTINGS: { resource: 'adty_site', action: 'settings' },
  CHANNEL_READ: { resource: 'channel', action: 'read' },
  CHANNEL_OPERATE: { resource: 'channel', action: 'operate' },
} as const satisfies Record<string, AdtyPermission>

/** True for root; for admins, true when the permission is granted. */
export function canAdmin(
  user: AuthUser | null | undefined,
  permission: AdtyPermission
): boolean {
  if (!user || user.role < ROLE.ADMIN) return false
  return hasPermission(user, permission.resource, permission.action)
}

/** Whether the user may see site-wide logs, tasks and statistics. */
export function canViewAllLogs(user: AuthUser | null | undefined): boolean {
  return canAdmin(user, ADTY_PERMISSIONS.LOG_READ)
}

export type AdtyPermissionPreset = {
  id: 'support' | 'operations' | 'finance'
  labelKey: string
  permissions: AdtyPermission[]
}

const SUPPORT: AdtyPermission[] = [
  ADTY_PERMISSIONS.USER_READ,
  ADTY_PERMISSIONS.LOG_READ,
  ADTY_PERMISSIONS.FINANCE_READ,
  ADTY_PERMISSIONS.REFERRAL_READ,
]

/** Presets replace the whole matrix when applied; they can be edited afterwards. */
export const ADTY_PERMISSION_PRESETS: AdtyPermissionPreset[] = [
  { id: 'support', labelKey: 'Support', permissions: SUPPORT },
  {
    id: 'operations',
    labelKey: 'Operations',
    permissions: [
      ...SUPPORT,
      ADTY_PERMISSIONS.USER_WRITE,
      ADTY_PERMISSIONS.CHANNEL_READ,
      ADTY_PERMISSIONS.CHANNEL_OPERATE,
      ADTY_PERMISSIONS.MODEL_READ,
      ADTY_PERMISSIONS.MODEL_WRITE,
      ADTY_PERMISSIONS.PAYMENT_READ,
      ADTY_PERMISSIONS.PAYMENT_OPERATE,
    ],
  },
  {
    id: 'finance',
    labelKey: 'Finance',
    permissions: [
      ADTY_PERMISSIONS.USER_READ,
      ADTY_PERMISSIONS.USER_QUOTA,
      ADTY_PERMISSIONS.FINANCE_READ,
      ADTY_PERMISSIONS.FINANCE_MANUAL_CREDIT,
      ADTY_PERMISSIONS.FINANCE_REFUND,
      ADTY_PERMISSIONS.REFERRAL_READ,
      ADTY_PERMISSIONS.REFERRAL_REVIEW,
      ADTY_PERMISSIONS.REFERRAL_PAYOUT,
      ADTY_PERMISSIONS.REFERRAL_ADJUST,
    ],
  },
]

/** Builds a full matrix for the catalog with only the preset's permissions on. */
export function applyPermissionPreset(
  catalog: PermissionCatalog,
  preset: AdtyPermissionPreset
): AdminPermissionMatrix {
  const matrix: AdminPermissionMatrix = {}
  for (const resource of catalog.resources) {
    const actions: Record<string, boolean> = {}
    for (const action of resource.actions) {
      actions[action.action] = preset.permissions.some(
        (permission) =>
          permission.resource === resource.resource &&
          permission.action === action.action
      )
    }
    matrix[resource.resource] = actions
  }
  return matrix
}
