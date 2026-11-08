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
import { afterEach, expect, it, vi } from 'vitest'

import { handoffOAuthLogin } from '../oauth-handoff'

afterEach(() =>
  document
    .querySelectorAll('meta[name="referrer"]')
    .forEach((meta) => meta.remove())
)

it('replaces the callback with the server handoff after suppressing the document referrer', () => {
  const destination =
    'https://customer.example/oauth/discord?code=private&state=flow'
  const replace = vi.fn(() =>
    expect(document.querySelector('meta[name="referrer"]')).toHaveAttribute(
      'content',
      'no-referrer'
    )
  )

  expect(
    handoffOAuthLogin({ oauth_handoff_url: destination }, 'discord', {
      document,
      replace,
    })
  ).toBe(true)
  expect(replace).toHaveBeenCalledWith(destination)
})

it.each([
  'javascript:alert(1)',
  'https://user:secret@site.example/oauth/discord',
  'https://site.example/oauth/github',
  'https://site.example/sign-in',
  '//site.example/oauth/discord',
])('rejects an unsafe handoff %s without navigation', (destination) => {
  const replace = vi.fn()

  expect(() =>
    handoffOAuthLogin({ oauth_handoff_url: destination }, 'discord', {
      document,
      replace,
    })
  ).toThrow()
  expect(replace).not.toHaveBeenCalled()
})

it('keeps an ordinary login result on the current origin', () => {
  const replace = vi.fn()

  expect(
    handoffOAuthLogin({ access_token: 'login-result' }, 'discord', {
      document,
      replace,
    })
  ).toBe(false)
  expect(replace).not.toHaveBeenCalled()
})
