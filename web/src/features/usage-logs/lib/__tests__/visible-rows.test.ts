import { describe, expect, it } from 'vitest'

import { visibleRowRange } from '../visible-rows'

describe('visible log rows', () => {
  it('uses the reserved table height before the scroll container is measured', () => {
    expect(visibleRowRange(20, 52, 0, 520)).toEqual({ start: 0, end: 10 })
  })

  it('renders only the rows that fit the measured viewport', () => {
    expect(visibleRowRange(20, 52, 520)).toEqual({ start: 0, end: 10 })
  })
})
