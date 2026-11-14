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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { EarningsPanel } from '../components/earnings-panel'
import { OverviewPanel } from '../components/overview-panel'
import { OwnerVerificationProvider } from '../components/owner-verification'
import { PricingPanel } from '../components/pricing-panel'
import { SiteSettingsPanel } from '../components/site-settings-panel'
import type { OwnerEarnings, OwnerSite, SitePrice } from '../types'

const api = vi.hoisted(() => ({
  getOwnerStats: vi.fn(),
  getOwnerEarnings: vi.fn(),
  savePricing: vi.fn(),
  downloadExport: vi.fn(),
  getSenderDomain: vi.fn(),
  removeSenderDomain: vi.fn(),
  uploadLogo: vi.fn(),
  saveOwnerSite: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

// The step-up dialog itself is covered by the secure-verification tests; here
// we only control what proof (if any) the owner's verification returns.
const verification = vi.hoisted(() => ({ requestVerification: vi.fn() }))
vi.mock('@/features/auth/secure-verification', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/auth/secure-verification')
  >()),
  useSecureVerification: () => ({
    requestVerification: verification.requestVerification,
    requestLoginVerification: vi.fn(),
    cancel: vi.fn(),
    isActive: false,
    dialogProps: {
      state: { phase: 'idle' },
      passkeyDomains: null,
      onCancel: vi.fn(),
      onRetry: vi.fn(),
      onInputChange: vi.fn(),
      onVerify: vi.fn(),
    },
  }),
  SecureVerificationDialog: () => null,
}))

function price(overrides: Partial<SitePrice>): SitePrice {
  return {
    id: 1,
    site_id: 7,
    user_id: 0,
    group: '',
    model: '',
    markup: '1.2',
    pending_markup: '0',
    pending_at: 0,
    version: 3,
    ...overrides,
  }
}

function ownerSite(overrides: Partial<OwnerSite> = {}): OwnerSite {
  return {
    id: 7,
    status: 'active',
    subdomain: 'shop',
    root_domain: 'sites.example',
    host: 'shop.sites.example',
    base_url: 'https://shop.sites.example',
    cost_factor: '0.8',
    markup_min: '1',
    markup_max: '3',
    markup_increase_delay_hours: 24,
    custom_domain_enabled: false,
    sender_domain_enabled: false,
    brand: {
      name: '',
      logo_url: '',
      tagline: '',
      notice: '',
      support_contact: '',
    },
    bonus_rates: {},
    seo_index: false,
    agreement_version: '3',
    agreement_at: 1_700_000_000,
    prices: [price({})],
    pricing_version: 3,
    domains: [],
    visibility: {},
    ...overrides,
  }
}

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>{node}</OwnerVerificationProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  api.savePricing.mockResolvedValue({
    version: 4,
    immediate: [''],
    pending: {},
  })
  api.removeSenderDomain.mockResolvedValue(undefined)
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('PricingPanel', () => {
  it('lists a saved increase that is not in effect yet with its new markup', () => {
    render(
      wrap(
        <PricingPanel
          site={ownerSite({
            prices: [
              price({}),
              price({
                id: 2,
                group: 'vip',
                markup: '1.5',
                pending_markup: '2',
                pending_at: 1_800_000_000,
              }),
            ],
          })}
        />
      )
    )
    const rows = screen.getAllByTestId('pending-increase')
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveTextContent('vip: 1.5 → 2')
    expect(screen.getByText(/after 24 hours/)).toBeInTheDocument()
  })

  it('shows no pending section when nothing is waiting', () => {
    render(wrap(<PricingPanel site={ownerSite()} />))
    expect(screen.queryByTestId('pending-increase')).not.toBeInTheDocument()
  })

  it('sends the proof for site-wide pricing with the current version', async () => {
    verification.requestVerification.mockResolvedValue({
      proof_token: 'proof-1',
    })
    render(wrap(<PricingPanel site={ownerSite()} />))
    await userEvent.clear(screen.getByLabelText('Default markup'))
    await userEvent.type(screen.getByLabelText('Default markup'), '1.1')
    await userEvent.click(screen.getByRole('button', { name: 'Save prices' }))

    await waitFor(() => expect(api.savePricing).toHaveBeenCalled())
    expect(verification.requestVerification).toHaveBeenCalledWith(
      expect.objectContaining({
        scope: 'site.owner.manage',
        context: { action: 'pricing', target: 0 },
      })
    )
    expect(api.savePricing).toHaveBeenCalledWith(
      expect.objectContaining({ default_markup: '1.1', expected_version: 3 }),
      'proof-1'
    )
  })

  it('does not save when the owner cancels the verification', async () => {
    verification.requestVerification.mockResolvedValue(null)
    render(wrap(<PricingPanel site={ownerSite()} />))
    await userEvent.click(screen.getByRole('button', { name: 'Save prices' }))
    await waitFor(() =>
      expect(verification.requestVerification).toHaveBeenCalled()
    )
    expect(api.savePricing).not.toHaveBeenCalled()
  })

  it('does not lock the form for a leftover paused site', () => {
    render(wrap(<PricingPanel site={ownerSite({ status: 'paused' })} />))
    expect(screen.getByRole('button', { name: 'Save prices' })).toBeEnabled()
    expect(screen.getByLabelText('Default markup')).toBeEnabled()
  })

  it('locks the form while the site is closing', () => {
    render(wrap(<PricingPanel site={ownerSite({ status: 'closing' })} />))
    expect(screen.getByRole('button', { name: 'Save prices' })).toBeDisabled()
    expect(screen.getByLabelText('Default markup')).toBeDisabled()
  })

  it('lists a pending independent model markup and can remove it to restore global', async () => {
    verification.requestVerification.mockResolvedValue({
      proof_token: 'proof-model',
    })
    render(
      wrap(
        <PricingPanel
          site={ownerSite({
            prices: [
              price({}),
              price({
                id: 3,
                model: 'gpt-4',
                markup: '1.5',
                pending_markup: '1.8',
                pending_at: 1_800_000_000,
              }),
            ],
          })}
        />
      )
    )
    const rows = screen.getAllByTestId('pending-increase')
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveTextContent('gpt-4: 1.5 → 1.8')
    expect(screen.getByDisplayValue('gpt-4')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Remove' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save prices' }))
    await waitFor(() => expect(api.savePricing).toHaveBeenCalled())
    expect(api.savePricing).toHaveBeenCalledWith(
      expect.objectContaining({
        model_markups: { 'gpt-4': '0' },
        expected_version: 3,
      }),
      'proof-model'
    )
  })
})

describe('OverviewPanel', () => {
  it('does not show a period or cumulative top-up total', async () => {
    api.getOwnerStats.mockResolvedValue({
      from: '2026-10-15',
      to: '2026-11-14',
      total: {
        day: '',
        new_customers: 2,
        paid_customers: 1,
        active_customers: 1,
        topup_usd: '88',
        revenue_usd: '12',
        platform_usd: '8',
        earning_usd: '4',
        bonus_usd: '0',
        refund_usd: '0',
        skipped_usd: '0',
      },
      series: [],
    })
    api.getOwnerEarnings.mockResolvedValue({
      balances: {
        pending_usd: '4',
        skipped_usd: '0',
        available_usd: '0',
        frozen_usd: '0',
        withdrawn_usd: '0',
      },
      page: { page: 1, page_size: 20, total: 0, items: [] },
    })
    render(wrap(<OverviewPanel />))
    expect(await screen.findByText('Customer spend')).toBeInTheDocument()
    expect(screen.getByText('New customers')).toBeInTheDocument()
    expect(screen.getByText('Paying customers')).toBeInTheDocument()
    expect(screen.getByText('$12.00')).toBeInTheDocument()
    expect(screen.queryByText('Top-ups')).not.toBeInTheDocument()
    expect(screen.queryByText('$88.00')).not.toBeInTheDocument()
  })
})

describe('EarningsPanel', () => {
  const earnings: OwnerEarnings = {
    balances: {
      pending_usd: '12.5',
      skipped_usd: '0',
      available_usd: '-8',
      frozen_usd: '0',
      withdrawn_usd: '30',
    },
    page: {
      page: 1,
      page_size: 20,
      total: 2,
      items: [
        {
          day: '2026-10-01',
          user_id: 5,
          kind: 'usage',
          revenue_usd: '125',
          earning_usd: '45',
          status: 'available',
          settled_at: 1_700_000_000,
        },
        {
          day: '2026-10-01+1',
          user_id: 5,
          kind: 'bonus',
          revenue_usd: '0',
          earning_usd: '-5',
          status: 'pending',
          settled_at: 0,
        },
      ],
    },
  }

  it('marks negative balances and negative rows in red, and leaves positive ones alone', async () => {
    api.getOwnerEarnings.mockResolvedValue(earnings)
    render(wrap(<EarningsPanel />))

    const negativeRow = await screen.findByText('$-5.00')
    expect(negativeRow).toHaveAttribute('data-negative', 'true')
    expect(screen.getByText('$45.00')).not.toHaveAttribute('data-negative')
    expect(screen.getByText('$-8.00')).toHaveAttribute('data-negative', 'true')
    expect(screen.getByText('$12.50')).not.toHaveAttribute('data-negative')
    expect(screen.getAllByText('2026-10-01')).toHaveLength(2)
  })
})

describe('Sender domain', () => {
  const pending = {
    site_id: 7,
    domain: 'mail.shop.example',
    status: 'pending',
    records: JSON.stringify([
      {
        record: 'SPF',
        name: 'send.mail.shop.example',
        type: 'TXT',
        value: 'v=spf1 include:amazonses.com ~all',
        ttl: 'Auto',
        status: 'not_started',
      },
    ]),
    disabled_by: 0,
    last_checked_at: 0,
    created_at: 0,
  }

  it('shows Not available yet when the platform has sender domains switched off', () => {
    render(
      wrap(
        <SiteSettingsPanel site={ownerSite({ sender_domain_enabled: false })} />
      )
    )
    expect(screen.getByText('Not available yet')).toBeInTheDocument()
    expect(api.getSenderDomain).not.toHaveBeenCalled()
  })

  it('lists the DNS records to add while the domain waits for verification', async () => {
    api.getSenderDomain.mockResolvedValue(pending)
    render(
      wrap(
        <SiteSettingsPanel site={ownerSite({ sender_domain_enabled: true })} />
      )
    )
    const table = await screen.findByRole('table', {
      name: 'DNS records to add',
    })
    expect(table).toHaveTextContent('send.mail.shop.example')
    expect(table).toHaveTextContent('v=spf1 include:amazonses.com ~all')
    expect(screen.getByText('Waiting for DNS')).toBeInTheDocument()
  })

  it('hides the records once the domain is verified', async () => {
    api.getSenderDomain.mockResolvedValue({ ...pending, status: 'verified' })
    render(
      wrap(
        <SiteSettingsPanel site={ownerSite({ sender_domain_enabled: true })} />
      )
    )
    expect(await screen.findByText('Verified')).toBeInTheDocument()
    expect(
      screen.queryByRole('table', { name: 'DNS records to add' })
    ).not.toBeInTheDocument()
  })

  it('asks for a sender.delete proof bound to the site before deleting', async () => {
    api.getSenderDomain.mockResolvedValue(pending)
    verification.requestVerification.mockResolvedValue({
      proof_token: 'proof-s',
    })
    render(
      wrap(
        <SiteSettingsPanel site={ownerSite({ sender_domain_enabled: true })} />
      )
    )
    const name = await screen.findByText('mail.shop.example')
    const card = name.closest('div.rounded-md') as HTMLElement
    await userEvent.click(within(card).getByRole('button', { name: 'Delete' }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Continue' })
    )

    await waitFor(() =>
      expect(api.removeSenderDomain).toHaveBeenCalledWith('proof-s')
    )
    expect(verification.requestVerification).toHaveBeenCalledWith(
      expect.objectContaining({
        context: { action: 'sender.delete', target: 7 },
      })
    )
  })
})

describe('Brand logo', () => {
  it('saves the uploaded logo address with the brand', async () => {
    api.uploadLogo.mockResolvedValue('/uploads/site-assets/site-7-abc.png')
    api.saveOwnerSite.mockResolvedValue(ownerSite())
    render(wrap(<SiteSettingsPanel site={ownerSite()} />))
    const file = new File(['png'], 'logo.png', { type: 'image/png' })
    await userEvent.upload(screen.getByLabelText('Logo'), file)
    expect(await screen.findByRole('img', { name: 'Logo' })).toHaveAttribute(
      'src',
      '/uploads/site-assets/site-7-abc.png'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Save brand' }))
    await waitFor(() =>
      expect(api.saveOwnerSite).toHaveBeenCalledWith({
        brand: expect.objectContaining({
          logo_url: '/uploads/site-assets/site-7-abc.png',
        }),
      })
    )
  })
})
