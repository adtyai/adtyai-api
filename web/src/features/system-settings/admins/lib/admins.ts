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

/** A36: POST /api/admin/admins helpers (pure, unit-tested). */

export const ADMIN_ROLE = 10

export type AdminCreateForm = {
  username: string
  password: string
  confirmPassword: string
  displayName: string
  email: string
}

export type AdminCreatePayload = {
  username: string
  password: string
  display_name?: string
  email?: string
  role: typeof ADMIN_ROLE
  confirm: true
}

export const EMPTY_ADMIN_FORM: AdminCreateForm = {
  username: '',
  password: '',
  confirmPassword: '',
  displayName: '',
  email: '',
}

export function buildAdminCreatePayload(
  form: AdminCreateForm
): AdminCreatePayload {
  const payload: AdminCreatePayload = {
    username: form.username.trim(),
    password: form.password,
    role: ADMIN_ROLE,
    confirm: true,
  }
  const dn = form.displayName.trim()
  if (dn) payload.display_name = dn
  const email = form.email.trim()
  if (email) payload.email = email
  return payload
}

export type AdminFormErrors = Partial<Record<keyof AdminCreateForm, string>>

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** Returns i18n keys per field; mirrors backend model validate tags. */
export function validateAdminForm(form: AdminCreateForm): AdminFormErrors {
  const e: AdminFormErrors = {}
  const u = form.username.trim()
  if (!u) e.username = 'Username is required'
  else if (u.length > 20) e.username = 'Username must be at most 20 characters'
  if (!form.password) e.password = 'Password is required'
  else if (form.password.length < 8 || form.password.length > 128) {
    e.password = 'Password must be 8-128 characters'
  }
  if (form.confirmPassword !== form.password) {
    e.confirmPassword = 'Passwords do not match'
  }
  if (form.displayName.trim().length > 20) {
    e.displayName = 'Display name must be at most 20 characters'
  }
  const email = form.email.trim()
  if (email) {
    if (email.length > 50) e.email = 'Email must be at most 50 characters'
    else if (!EMAIL_RE.test(email)) e.email = 'Invalid email address'
  }
  return e
}

/** Map status/code to a fixed i18n key; never surfaces raw backend text. */
export function adminCreateErrorKey(status?: number, code?: string): string {
  if (status === 403 || code === 'root_required') {
    return 'Only Root can create administrators'
  }
  switch (code) {
    case 'confirm_required':
      return 'Second confirmation is required'
    case 'role_not_allowed':
      return 'Only the Admin role can be created'
    case 'username_taken':
      return 'Username already exists'
    case 'invalid_params':
      return 'Invalid input, please check the fields'
    case 'bad_request':
      return 'Invalid request'
    case 'db_error':
    case 'create_failed':
      return 'Failed to create administrator, please try again later'
  }
  if (status === 400) return 'Invalid input, please check the fields'
  return 'Failed to create administrator, please try again later'
}
