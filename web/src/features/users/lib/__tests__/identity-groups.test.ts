import { describe, expect, it } from 'vitest'

import { identityGroupChoices } from '../user-form'

describe('identityGroupChoices', () => {
  it('offers only public billing groups and keeps the current identity', () => {
    expect(
      identityGroupChoices(
        ['default', 'vip', 'auto'],
        { default: 'Default', stale: 'Old' },
        ''
      )
    ).toEqual(['default'])
    expect(
      identityGroupChoices(
        ['default', 'vip', 'auto'],
        { default: 'Default' },
        'vip'
      )
    ).toEqual(['default', 'vip'])
  })
})
