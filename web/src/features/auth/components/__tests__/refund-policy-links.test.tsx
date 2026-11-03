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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import type { SystemStatus } from '../../types'
import { LegalConsent } from '../legal-consent'
import { TermsFooter } from '../terms-footer'

const http = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  api: { get: http.get },
}))

function withSite(refundPolicyEnabled: boolean) {
  http.get.mockResolvedValue({
    data: {
      success: true,
      data: { refund_policy_enabled: refundPolicyEnabled },
    },
  })
}

function renderWithQuery(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const bothAgreements: SystemStatus = {
  user_agreement_enabled: true,
  privacy_policy_enabled: true,
}

describe('refund policy links', () => {
  it('adds the refund policy to the consent text when the site enables it', async () => {
    withSite(true)
    renderWithQuery(
      <LegalConsent
        status={bothAgreements}
        checked={false}
        onCheckedChange={vi.fn()}
      />
    )
    expect(
      await screen.findByRole('link', { name: 'Refund Policy' })
    ).toHaveAttribute('href', '/refund-policy')
    expect(screen.getByRole('link', { name: 'User Agreement' })).toBeVisible()
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toBeVisible()
  })

  it('leaves the consent text unchanged when the site disables it', async () => {
    withSite(false)
    renderWithQuery(
      <LegalConsent
        status={bothAgreements}
        checked={false}
        onCheckedChange={vi.fn()}
      />
    )
    await screen.findByRole('link', { name: 'User Agreement' })
    await vi.waitFor(() => expect(http.get).toHaveBeenCalled())
    expect(
      screen.queryByRole('link', { name: 'Refund Policy' })
    ).not.toBeInTheDocument()
  })

  it('does not show the consent checkbox for the refund policy alone', async () => {
    withSite(true)
    renderWithQuery(
      <LegalConsent status={{}} checked={false} onCheckedChange={vi.fn()} />
    )
    await vi.waitFor(() => expect(http.get).toHaveBeenCalled())
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('adds the refund policy to the sign-in terms when the site enables it', async () => {
    withSite(true)
    renderWithQuery(<TermsFooter status={{ user_agreement_enabled: true }} />)
    expect(
      await screen.findByRole('link', { name: 'Refund Policy' })
    ).toHaveAttribute('href', '/refund-policy')
    expect(screen.getByRole('link', { name: 'User Agreement' })).toBeVisible()
  })
})
