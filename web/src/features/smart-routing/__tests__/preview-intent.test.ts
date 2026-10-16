import { describe, expect, it } from 'vitest'

import { previewIntentFor } from '../lib/preview-intent'

const cur = { connectivity: 30, price: 25, speed: 20, stability: 15, authenticity: 10 } as never
const nxt = { connectivity: 40, price: 20, speed: 20, stability: 10, authenticity: 10 } as never

describe('previewIntentFor (N-P1-3)', () => {
  it('Preview & apply → update_weights on active scheme', () => {
    expect(previewIntentFor({ activeScheme: 'balanced_default', currentWeights: cur, nextWeights: nxt })).toEqual({ kind: 'update_weights', scheme_name: 'balanced_default' })
  })
  it('switch scheme with different weights → put_config', () => {
    expect(previewIntentFor({ activeScheme: 'balanced_default', currentWeights: cur, nextWeights: nxt, schemeName: 'fast' })).toEqual({ kind: 'put_config', scheme_name: 'fast' })
  })
  it('switch scheme with same weights → activate_scheme', () => {
    expect(previewIntentFor({ activeScheme: 'balanced_default', currentWeights: cur, nextWeights: cur, schemeName: 'fast' }).kind).toBe('activate_scheme')
  })
  it('clicking the already-active scheme → update_weights', () => {
    expect(previewIntentFor({ activeScheme: 'fast', currentWeights: cur, nextWeights: nxt, schemeName: 'fast' })).toEqual({ kind: 'update_weights', scheme_name: 'fast' })
  })
})

describe('validWeights (N-P2-4 FE guard)', () => {
  it('rejects negative and all-zero', async () => {
    const { validWeights } = await import('../lib/preview-intent')
    expect(validWeights(cur)).toBe(true)
    expect(validWeights({ ...(cur as object), price: -1 } as never)).toBe(false)
    expect(validWeights({ connectivity: 0, price: 0, speed: 0, stability: 0, authenticity: 0 } as never)).toBe(false)
  })
})
