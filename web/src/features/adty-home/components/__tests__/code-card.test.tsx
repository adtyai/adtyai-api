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
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { CodeCard } from '../code-card'
import { MigrationSection } from '../migration-section'

describe('landing code card', () => {
  it('lets the client tabs shrink so Copy code stays in the card', () => {
    render(<CodeCard baseUrl='https://example.test/v1' />)

    const tabs = screen.getByRole('tablist')
    expect(tabs.className).toContain('min-w-0')
    expect(tabs.className).toContain('flex-1')
    expect(
      screen.getByRole('button', { name: 'Copy code' })
    ).toBeInTheDocument()
  })

  it('scrolls long snippets inside the card instead of widening the page', () => {
    render(<MigrationSection baseUrl='https://example.test/v1' />)

    expect(
      screen.getByRole('button', { name: 'Copy code' })
    ).toBeInTheDocument()
    expect(screen.getByTestId('landing-code').className).toContain(
      'overflow-x-auto'
    )
  })
})
