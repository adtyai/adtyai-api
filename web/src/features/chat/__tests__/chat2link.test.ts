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

import { chat2LinkOutcome } from '../lib/chat2link'

describe('chat2link', () => {
  it('stays pending while status is loading', () => {
    expect(chat2LinkOutcome([], true)).toBe('pending')
  })

  it('reports missing when loaded chats have no web preset', () => {
    expect(chat2LinkOutcome([], false)).toBe('missing')
    expect(chat2LinkOutcome([{ type: 'other' }], false)).toBe('missing')
  })

  it('is ready when a web preset exists', () => {
    expect(chat2LinkOutcome([{ type: 'web' }], false)).toBe('ready')
  })
})
