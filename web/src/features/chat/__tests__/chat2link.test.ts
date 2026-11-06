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
