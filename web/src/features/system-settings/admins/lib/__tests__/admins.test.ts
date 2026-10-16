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

import { describe, expect, it } from 'vitest'

import {
  ADMIN_ROLE,
  adminCreateErrorKey,
  buildAdminCreatePayload,
  EMPTY_ADMIN_FORM,
  validateAdminForm,
} from '../admins'

const valid = {
  username: ' alice ',
  password: 'secret123',
  confirmPassword: 'secret123',
  displayName: ' Alice ',
  email: '',
}

describe('buildAdminCreatePayload', () => {
  it('always sends confirm flag and role=Admin, trims fields', () => {
    const p = buildAdminCreatePayload(valid)
    expect(p).toEqual({
      username: 'alice',
      password: 'secret123',
      display_name: 'Alice',
      role: ADMIN_ROLE,
      confirm: true,
    })
    expect(ADMIN_ROLE).toBe(10)
    expect('confirmPassword' in p).toBe(false)
  })
  it('omits empty optional fields', () => {
    const p = buildAdminCreatePayload({
      ...valid,
      displayName: '  ',
      email: ' a@b.co ',
    })
    expect(p.display_name).toBeUndefined()
    expect(p.email).toBe('a@b.co')
  })
})

describe('validateAdminForm', () => {
  it('accepts a valid form', () => {
    expect(validateAdminForm(valid)).toEqual({})
  })
  it('flags required and length rules', () => {
    const e = validateAdminForm(EMPTY_ADMIN_FORM)
    expect(e.username).toBe('Username is required')
    expect(e.password).toBe('Password is required')
    expect(
      validateAdminForm({ ...valid, username: 'x'.repeat(21) }).username
    ).toBeTruthy()
    expect(
      validateAdminForm({
        ...valid,
        password: 'short',
        confirmPassword: 'short',
      }).password
    ).toBe('Password must be 8-128 characters')
    expect(
      validateAdminForm({ ...valid, displayName: 'y'.repeat(21) }).displayName
    ).toBeTruthy()
  })
  it('flags mismatched password and bad email', () => {
    expect(
      validateAdminForm({ ...valid, confirmPassword: 'other1234' })
        .confirmPassword
    ).toBe('Passwords do not match')
    expect(validateAdminForm({ ...valid, email: 'nope' }).email).toBe(
      'Invalid email address'
    )
  })
  it('flags email longer than 50 characters (backend max=50)', () => {
    const long = `${'a'.repeat(40)}@example.com` // 52 chars
    expect(long.length).toBeGreaterThan(50)
    expect(validateAdminForm({ ...valid, email: long }).email).toBe(
      'Email must be at most 50 characters'
    )
    const ok50 = `${'a'.repeat(38)}@example.com` // 50 chars
    expect(ok50.length).toBe(50)
    expect(validateAdminForm({ ...valid, email: ok50 }).email).toBeUndefined()
  })
})

describe('adminCreateErrorKey', () => {
  it('maps 403 regardless of code', () => {
    expect(adminCreateErrorKey(403)).toBe('Only Root can create administrators')
    expect(adminCreateErrorKey(403, 'root_required')).toBe(
      'Only Root can create administrators'
    )
  })
  it('maps 400 codes to fixed copy', () => {
    expect(adminCreateErrorKey(400, 'confirm_required')).toBe(
      'Second confirmation is required'
    )
    expect(adminCreateErrorKey(400, 'role_not_allowed')).toBe(
      'Only the Admin role can be created'
    )
    expect(adminCreateErrorKey(400, 'username_taken')).toBe(
      'Username already exists'
    )
    expect(adminCreateErrorKey(400, 'invalid_params')).toBe(
      'Invalid input, please check the fields'
    )
    expect(adminCreateErrorKey(400, 'create_failed')).toBe(
      'Failed to create administrator, please try again later'
    )
    expect(adminCreateErrorKey(400, 'pq: duplicate key value')).toBe(
      'Invalid input, please check the fields'
    )
  })
  it('falls back for 5xx / network', () => {
    expect(adminCreateErrorKey(500, 'db_error')).toBe(
      'Failed to create administrator, please try again later'
    )
    expect(adminCreateErrorKey()).toBe(
      'Failed to create administrator, please try again later'
    )
  })
})
