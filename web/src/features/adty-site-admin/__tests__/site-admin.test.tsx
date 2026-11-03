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

import { useAuthStore } from '@/stores/auth-store'

import { CloseSiteDialog } from '../components/close-site-dialog'
import { OverviewPanel } from '../components/overview-panel'
import { ReviewsPanel } from '../components/reviews-panel'
import { SettingsPanel } from '../components/settings-panel'
import { SiteDetailSheet } from '../components/site-detail-sheet'
import { SitesPanel } from '../components/sites-panel'
import { UsagePanel } from '../components/usage-panel'
import type { AdminSite, ResellerSettings } from '../types'

const api = vi.hoisted(() => ({
  getSites: vi.fn(),
  createSite: vi.fn(),
  getSitesOverview: vi.fn(),
  getSiteStats: vi.fn(),
  getFlaggedCustomers: vi.fn(),
  getSiteReports: vi.fn(),
  handleSiteReport: vi.fn(),
  setSiteStatus: vi.fn(),
  getResellerSettings: vi.fn(),
  saveResellerSettings: vi.fn(),
  getSite: vi.fn(),
  getSiteCustomers: vi.fn(),
  clearSiteNotice: vi.fn(),
  getResellerUsage: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

function site(overrides: Partial<AdminSite>): AdminSite {
  return {
    id: 1,
    owner_user_id: 10,
    owner_username: 'owner',
    subdomain: 'alpha',
    root_domain: 'sites.example',
    host: 'alpha.sites.example',
    status: 'active',
    cost_factor: null,
    max_markup: null,
    tier_cost_factor: null,
    tier_month: '',
    visibility: '',
    brand: '',
    contact_info: '',
    opened_by: 'apply',
    review_note: '',
    close_requested_at: 0,
    close_execute_at: 0,
    close_transfer_to: 0,
    close_claw_back: false,
    close_reason: '',
    closed_at: 0,
    customers: 0,
    created_at: 1_700_000_000,
    updated_at: 1_700_000_000,
    ...overrides,
  }
}

const settings: ResellerSettings = {
  enabled: false,
  root_domains: ['sites.example'],
  reserved_subdomains: ['www'],
  unknown_host_mode: 'main',
  cost_factor: null,
  min_cost_factor: null,
  cost_factor_tiers: [],
  max_markup: null,
  max_site_bonus_extra: null,
  trial_grant_max_usd: null,
  trial_grant_daily_usd: null,
  markup_increase_delay_hours: 24,
  settle_hour: 4,
  freeze_days: 7,
  close_notice_days: 7,
  site_login_only: true,
  custom_domain_enabled: false,
  sender_domain_enabled: false,
  invite_code_enabled: false,
  self_use_check_enabled: true,
  migration_check_enabled: true,
  fingerprint_window_days: 30,
  customer_lookup_per_hour: 20,
  max_custom_domains_per_site: 2,
  custom_domain_creates_per_hour: 3,
  visibility: { customer_email: 'masked' },
  cloudflare_apiToken: '',
  cloudflare_zone_id: '',
  cloudflare_account_id: '',
  cname_target: '',
  fallback_origin: '',
  turnstile_widgets: [{ id: 'w1', site_key: '0x4AAA' }],
  turnstile_widget_secret: null,
  cloudflare_token_configured: false,
  turnstile_secret_configured: { w1: false },
  cloudflare_configured: false,
  resend_configured: false,
  money_configured: false,
  site_agreement_version: 1,
  site_customer_terms_version: 1,
}

function asRoot() {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
}

function renderWith(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

function siteNames(): string[] {
  const table = screen.getByRole('table', { name: 'Reseller sites' })
  return within(table)
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('cell')[0].textContent ?? '')
}

beforeEach(() => {
  asRoot()
})

afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.clearAllMocks()
})

describe('site list', () => {
  beforeEach(() => {
    api.getSites.mockImplementation(
      async (params: { sort?: string; order?: string }) => {
        const byRevenue = [
          site({
            id: 2,
            host: 'beta.sites.example',
            customers: 40,
            month_revenue_usd: '100',
            month_topup_usd: '80',
            month_earning_usd: '20',
            growth_pct: 12.5,
          }),
          site({
            id: 1,
            host: 'alpha.sites.example',
            customers: 12,
            month_revenue_usd: '10',
            month_topup_usd: '8',
            month_earning_usd: '2',
            growth_pct: null,
          }),
        ]
        const items =
          params.sort === 'customers' && params.order === 'asc'
            ? [...byRevenue].reverse()
            : byRevenue
        return { page: 1, page_size: 20, total: items.length, items }
      }
    )
    api.createSite.mockResolvedValue(site({ id: 9, host: 'new.sites.example' }))
  })

  it('asks the server to sort by revenue and shows this month', async () => {
    renderWith(<SitesPanel onOpenSite={() => undefined} />)

    await screen.findByText('beta.sites.example')
    expect(api.getSites).toHaveBeenCalledWith(
      expect.objectContaining({ sort: 'revenue', order: 'desc', p: 1 })
    )
    expect(siteNames()).toEqual([
      'beta.sites.example',
      'alpha.sites.example',
    ])
    expect(screen.getByRole('cell', { name: '100' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: '12.5%' })).toBeInTheDocument()
  })

  it('sends the next sort to the server instead of reordering the page', async () => {
    const user = userEvent.setup()
    renderWith(<SitesPanel onOpenSite={() => undefined} />)
    await screen.findByText('beta.sites.example')

    await user.click(screen.getByRole('button', { name: /Customers/ }))
    await waitFor(() =>
      expect(api.getSites).toHaveBeenCalledWith(
        expect.objectContaining({ sort: 'customers', order: 'desc' })
      )
    )
    await user.click(screen.getByRole('button', { name: /Customers/ }))
    await waitFor(() =>
      expect(api.getSites).toHaveBeenCalledWith(
        expect.objectContaining({ sort: 'customers', order: 'asc' })
      )
    )
    expect(siteNames()[0]).toBe('alpha.sites.example')
  })

  it('opens a site through the create API', async () => {
    const user = userEvent.setup()
    renderWith(<SitesPanel onOpenSite={() => undefined} />)
    await screen.findByText('beta.sites.example')

    await user.type(screen.getByRole('textbox', { name: 'User ID' }), '15')
    await user.type(screen.getByRole('textbox', { name: 'Domain' }), 'shop')
    await user.click(
      screen.getByRole('button', { name: 'Open a reseller site' })
    )
    await waitFor(() =>
      expect(api.createSite).toHaveBeenCalledWith({
        user_id: 15,
        subdomain: 'shop',
      })
    )
  })
})

describe('overview and reviews', () => {
  it('shows hostname cost, hook failures and held sites from the overview API', async () => {
    api.getSitesOverview.mockResolvedValue({
      from: '2026-11-01',
      to: '2026-11-02',
      sites: { active: 1 },
      site_customers: 1,
      main_customers: 0,
      site_topup_usd: '10',
      main_topup_usd: '0',
      site_revenue_usd: '115',
      site_platform_usd: '92',
      site_earning_usd: '22',
      site_skipped_usd: '1',
      site_bonus_usd: '0',
      pending_reports: 1,
      flagged_customers: 1,
      pending_applications: 0,
      custom_hostnames: 2,
      hostname_monthly_fee_usd: '0',
      earning_hook_failures: 3,
      settlement_holds: [
        { site_id: 4, subdomain: 'big', settle_hold_reason: 'usage mismatch' },
      ],
    })
    renderWith(<OverviewPanel />)
    expect(await screen.findByText('115')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('big: usage mismatch')).toBeInTheDocument()
  })

  it('loads flagged customers and resolves a report', async () => {
    const user = userEvent.setup()
    api.getFlaggedCustomers.mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 1,
      items: [
        {
          user_id: 8,
          site_id: 4,
          risk_flag: 'self_use',
          risk_reason: 'same IP',
          reviewed: false,
          skipped_usd: '1.00',
        },
      ],
    })
    api.getSiteReports.mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 1,
      items: [
        { id: 6, site_id: 4, user_id: 8, reason: 'abuse', status: 'pending' },
      ],
    })
    api.handleSiteReport.mockResolvedValue({
      id: 6,
      site_id: 4,
      user_id: 8,
      reason: 'abuse',
      status: 'resolved',
    })
    renderWith(<ReviewsPanel />)
    expect(await screen.findByText('same IP')).toBeInTheDocument()
    expect(screen.getByText('abuse')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Resolve report' }))
    await waitFor(() =>
      expect(api.handleSiteReport).toHaveBeenCalledWith(6, {
        status: 'resolved',
        note: '',
      })
    )
  })
})

describe('close site dialog', () => {
  const others = [
    site({ id: 5, host: 'other.sites.example', status: 'active' }),
  ]

  it('cannot be confirmed until a customer destination is chosen', async () => {
    const user = userEvent.setup()
    renderWith(
      <CloseSiteDialog
        site={site({ id: 1 })}
        transferTargets={others}
        open
        onOpenChange={() => undefined}
      />
    )

    const confirm = screen.getByRole('button', { name: 'Close site' })
    expect(confirm).toBeDisabled()
    expect(
      screen.getByText('Choose where the customers go.')
    ).toBeInTheDocument()

    await user.click(
      screen.getByRole('radio', { name: 'Move to the main site' })
    )
    expect(confirm).toBeEnabled()
  })

  it('sends the chosen site as transfer_to', async () => {
    const user = userEvent.setup()
    api.setSiteStatus.mockResolvedValue(site({ status: 'closing' }))
    renderWith(
      <CloseSiteDialog
        site={site({ id: 1 })}
        transferTargets={others}
        open
        onOpenChange={() => undefined}
      />
    )

    await user.click(
      screen.getByRole('radio', { name: 'Move to another site' })
    )
    await user.selectOptions(
      screen.getByRole('combobox', { name: 'Target site' }),
      '5'
    )
    await user.type(screen.getByLabelText('Reason'), 'owner request')
    await user.click(screen.getByRole('button', { name: 'Close site' }))

    await waitFor(() =>
      expect(api.setSiteStatus).toHaveBeenCalledWith(1, {
        status: 'closed',
        transfer_to: 5,
        claw_back: false,
        immediate: false,
        reason: 'owner request',
      })
    )
  })

  it('needs a target site when moving to another site', async () => {
    const user = userEvent.setup()
    renderWith(
      <CloseSiteDialog
        site={site({ id: 1 })}
        transferTargets={others}
        open
        onOpenChange={() => undefined}
      />
    )

    await user.click(
      screen.getByRole('radio', { name: 'Move to another site' })
    )

    expect(screen.getByRole('button', { name: 'Close site' })).toBeDisabled()
  })
})

describe('reseller settings', () => {
  beforeEach(() => {
    api.getResellerSettings.mockResolvedValue(settings)
  })

  it('disables custom domains until Cloudflare is configured and says why', async () => {
    renderWith(<SettingsPanel canWrite />)

    const custom = await screen.findByRole('switch', { name: 'Custom domains' })
    expect(custom).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByText(
        'Fill in the Cloudflare API token, zone ID and CNAME target first.'
      )
    ).toBeInTheDocument()
  })

  it('enables custom domains once the token, zone ID and CNAME target are typed', async () => {
    const user = userEvent.setup()
    renderWith(<SettingsPanel canWrite />)
    await screen.findByRole('switch', { name: 'Custom domains' })

    await user.type(screen.getByLabelText('Cloudflare API token'), 'cf-token')
    await user.type(screen.getByLabelText('Cloudflare zone ID'), 'zone-1')
    await user.type(
      screen.getByLabelText('CNAME target'),
      'customers.sites.example'
    )

    expect(
      screen.getByRole('switch', { name: 'Custom domains' })
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('disables sender domains while the Resend API key is missing', async () => {
    renderWith(<SettingsPanel canWrite />)

    expect(
      await screen.findByRole('switch', { name: 'Site owner sender domains' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByText(
        'Fill in the Resend API key under System Settings > Email first.'
      )
    ).toBeInTheDocument()
  })

  it('disables the master switch while money settings are unset', async () => {
    renderWith(<SettingsPanel canWrite />)

    expect(
      await screen.findByRole('switch', { name: 'Reseller sites' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getAllByText('Not configured').length).toBeGreaterThan(0)
  })

  it('never shows saved secrets and marks them as configured', async () => {
    api.getResellerSettings.mockResolvedValue({
      ...settings,
      cloudflare_token_configured: true,
      turnstile_secret_configured: { w1: true },
    })
    renderWith(<SettingsPanel canWrite />)

    const token = await screen.findByLabelText('Cloudflare API token')
    expect(token).toHaveValue('')
    expect(token).toHaveAttribute('type', 'password')
    expect(token).toHaveAttribute(
      'placeholder',
      'Saved. Enter a new value to replace it'
    )
    expect(screen.getByLabelText('Secret of widget 1')).toHaveValue('')
  })

  it('sends only a typed secret and the changed keys', async () => {
    const user = userEvent.setup()
    api.saveResellerSettings.mockResolvedValue(settings)
    renderWith(<SettingsPanel canWrite />)
    await screen.findByLabelText('Cloudflare API token')

    await user.type(screen.getByLabelText('Secret of widget 1'), 'sec-1')
    await user.clear(screen.getByLabelText('Freeze days'))
    await user.type(screen.getByLabelText('Freeze days'), '10')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(api.saveResellerSettings).toHaveBeenCalledWith({
        freeze_days: 10,
        turnstile_widget_secret: { w1: 'sec-1' },
      })
    )
  })
})

describe('SiteDetailSheet notice takedown', () => {
  const withNotice = site({
    brand: JSON.stringify({ name: 'Alpha', notice: 'Visit evil.example' }),
  })

  function openSheet(target: AdminSite) {
    api.getSite.mockResolvedValue({ site: target, audits: [] })
    api.getSiteCustomers.mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 0,
      items: [],
    })
    api.clearSiteNotice.mockResolvedValue({ site_id: target.id, cleared: true })
    api.getSiteStats.mockResolvedValue({
      stats: {
        from: '2026-11-01',
        to: '2026-11-02',
        series: [],
        total: {
          day: '',
          revenue_usd: '15',
          platform_usd: '12',
          earning_usd: '3',
          topup_usd: '0',
        },
      },
      breakdown: {},
      monthly: [],
      withdrawn_usd: '12.5',
    })
    renderWith(<SiteDetailSheet siteId={target.id} onOpenChange={() => {}} />)
  }

  it('takes the notice down after confirming', async () => {
    openSheet(withNotice)
    expect(await screen.findByText('Visit evil.example')).toBeInTheDocument()
    expect(await screen.findByText('15')).toBeInTheDocument()
    expect(screen.getByText('12.5')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Take down notice' })
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Continue' })
    )
    await waitFor(() => expect(api.clearSiteNotice).toHaveBeenCalledWith(1, ''))
  })

  it('offers no takedown when the site has no notice', async () => {
    openSheet(site({ brand: JSON.stringify({ name: 'Alpha' }) }))
    await screen.findByRole('button', { name: 'Close site' })
    expect(
      screen.queryByRole('button', { name: 'Take down notice' })
    ).not.toBeInTheDocument()
  })

  it('hides the takedown from admins without the adjust permission', async () => {
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'viewer',
      role: 10,
      permissions: { admin_permissions: { adty_site: { read: true } } },
    })
    openSheet(withNotice)
    expect(await screen.findByText('alpha.sites.example')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Take down notice' })
    ).not.toBeInTheDocument()
  })
})

describe('UsagePanel', () => {
  const usage = {
    sender: {
      in_use: 3,
      verified: 1,
      failed: 1,
      disabled: 1,
      quota_full_at: 0,
    },
    widgets: [
      { id: 'w1', site_key: '0xA', used: 10, limit: 10 },
      { id: 'w2', site_key: '0xB', used: 7, limit: 10 },
    ],
    widget_slots_left: 3,
    widget_slots_low: true,
    unassigned_domains: 0,
  }

  it('lists each widget as used / limit and warns when fewer than five slots are left', async () => {
    api.getResellerUsage.mockResolvedValue(usage)
    renderWith(<UsagePanel />)
    const table = await screen.findByRole('table', {
      name: 'Turnstile widgets',
    })
    expect(within(table).getByText('10 / 10')).toBeInTheDocument()
    expect(within(table).getByText('7 / 10')).toBeInTheDocument()
    expect(
      screen.getByText(/Only 3 Turnstile domain slots are left/)
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/no Resend slots were left/)
    ).not.toBeInTheDocument()
  })

  it('shows the quota-full alert after a refused sender domain and no slot warning when there is room', async () => {
    api.getResellerUsage.mockResolvedValue({
      ...usage,
      sender: { ...usage.sender, quota_full_at: 1_700_000_000 },
      widget_slots_left: 13,
      widget_slots_low: false,
    })
    renderWith(<UsagePanel />)
    expect(
      await screen.findByText(/no Resend slots were left/)
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/Turnstile domain slots are left/)
    ).not.toBeInTheDocument()
  })
})
