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
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  ReferralAffiliate,
  ReferralWithdrawal,
} from '@/features/referral/types'
import { useAuthStore } from '@/stores/auth-store'

import type { ReferralSettings } from '../api'
import { AffiliatePanel } from '../components/affiliate-panel'
import { SettingsPanel } from '../components/settings-panel'
import { WithdrawalPanel } from '../components/withdrawal-panel'
import { useAdminReferralBadge } from '../hooks/use-admin-referral-badge'
import { AdminReferral } from '../index'
import { buildSettingsUpdate, toDrafts } from '../lib/settings'

const api = vi.hoisted(() => ({
  getAdminWithdrawals: vi.fn(),
  reviewWithdrawal: vi.fn(),
  payWithdrawal: vi.fn(),
  getAffiliates: vi.fn(),
  reviewPartner: vi.fn(),
  runSettlement: vi.fn(),
  getReferralBadges: vi.fn(),
  getReferralOverview: vi.fn(),
  getReferralSettings: vi.fn(),
  saveReferralSettings: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

const http = vi.hoisted(() => ({
  get: vi.fn(),
  counts: {} as Record<string, number>,
  failNext: false,
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  api: { get: http.get },
}))

function withdrawal(
  overrides: Partial<ReferralWithdrawal>
): ReferralWithdrawal {
  return {
    id: 1,
    affiliate_id: 1,
    user_id: 7,
    amount: 150,
    fee_amount: 2,
    net_amount: 148,
    account_type: 'usdt',
    account_name: '',
    account_no: 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
    account_network: 'TRC20',
    qr_image_url: '',
    cny_rate: '',
    cny_amount: '',
    admin_note: '',
    payment_txn_no: '',
    payment_proof_url: '',
    status: 'pending',
    reject_reason: '',
    submitted_at: 1_700_000_000,
    paid_at: 0,
    ...overrides,
  }
}

const settings: ReferralSettings = {
  enabled: false,
  default_rate: null,
  cookie_ttl_days: 30,
  freeze_days: 7,
  min_withdraw_usd: null,
  usdt_withdraw_fee: null,
  cancel_window_minutes: 30,
  withdraw_cny_rate: '',
  withdraw_cny_rate_updated_at: 0,
  self_invite_window_days: 30,
  batch_register_threshold: 3,
  batch_register_window_hours: 24,
}

function asAdmin(referral: Record<string, boolean>) {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'ops',
    role: 10,
    permissions: { admin_permissions: { adty_referral: referral } },
  })
}

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

function affiliate(
  overrides: Partial<ReferralAffiliate> = {}
): ReferralAffiliate {
  return {
    id: 1,
    user_id: 9,
    invite_code: 'ABC123',
    status: 'approved',
    partner_status: 'pending',
    applicant_note: 'Please review',
    rate_override: null,
    acquisition_enabled: true,
    settlement_enabled: true,
    withdrawal_enabled: true,
    risk_reason: '',
    risk_note: '',
    site_owner_since: 0,
    created_at: 1_700_000_000,
    ...overrides,
  }
}

function serveSidebarBadges(counts: Record<string, number>) {
  http.counts = counts
}

function badgeGets(): number {
  return http.get.mock.calls.filter((call) => call[0] === '/api/adty/badges')
    .length
}

function ReferralSidebarBadge() {
  const badge = useAdminReferralBadge()
  return <div data-testid='referral-sidebar-badge'>{badge ?? ''}</div>
}

function renderWithReferralBadge(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  function Shell(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <ReferralSidebarBadge />
        {props.children}
      </QueryClientProvider>
    )
  }
  return { client, ...render(ui, { wrapper: Shell }) }
}

beforeEach(() => {
  http.failNext = false
  http.counts = {}
  http.get.mockImplementation(async (url: string) => {
    if (url === '/api/adty/badges') {
      if (http.failNext) {
        http.failNext = false
        throw new Error('badge query failed')
      }
      return {
        data: {
          success: true,
          data: { counts: { ...http.counts } },
        },
      }
    }
    return { data: { success: true, data: {} } }
  })
  api.reviewWithdrawal.mockResolvedValue(undefined)
  api.payWithdrawal.mockResolvedValue(undefined)
  api.reviewPartner.mockResolvedValue(undefined)
  api.runSettlement.mockResolvedValue(undefined)
  api.getAffiliates.mockResolvedValue({
    page: 1,
    page_size: 20,
    total: 0,
    items: [],
  })
  api.getReferralSettings.mockResolvedValue(settings)
  api.getReferralOverview.mockResolvedValue({
    affiliates: 0,
    bindings: 0,
    pending: '0',
    available: '0',
    frozen: '0',
    withdrawn: '0',
  })
  api.getReferralBadges.mockResolvedValue({
    partners: 0,
    pending_withdrawals: 0,
    approved_withdrawals: 0,
    total: 0,
  })
  api.getAdminWithdrawals.mockResolvedValue({
    page: 1,
    page_size: 20,
    total: 2,
    items: [
      withdrawal({}),
      withdrawal({
        id: 2,
        account_type: 'alipay',
        account_no: '',
        account_network: '',
        account_name: 'Zhang San',
        qr_image_url: '/api/referral-assets/qr.png',
        cny_rate: '7.1',
        cny_amount: '710.00',
        amount: 100,
        net_amount: 100,
        fee_amount: 0,
        status: 'approved',
      }),
    ],
  })
})

afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.clearAllMocks()
})

describe('WithdrawalPanel', () => {
  it('shows the chain, address and the exact amount to pay', async () => {
    asAdmin({ read: true, review: true, payout: true })
    render(<WithdrawalPanel canReview canPayout />, { wrapper })
    expect(await screen.findByText('TRC20')).toBeInTheDocument()
    expect(
      screen.getByText('TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t')
    ).toBeInTheDocument()
    const payouts = screen.getAllByTestId('payout').map((el) => el.textContent)
    expect(payouts).toEqual(['148.00 USDT', '¥710.00 (1 USD = 7.1 CNY)'])
    expect(screen.getByText('Zhang San')).toBeInTheDocument()
    expect(screen.getByAltText('Payment QR code')).toHaveAttribute(
      'src',
      '/api/referral-assets/qr.png'
    )
  })

  it('hides the payout button for an admin who can only review', async () => {
    asAdmin({ read: true, review: true })
    render(<WithdrawalPanel canReview canPayout={false} />, { wrapper })
    expect(
      await screen.findByRole('button', { name: 'Approve' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Mark as paid' })
    ).not.toBeInTheDocument()
  })
})

describe('SettingsPanel', () => {
  beforeEach(() => {
    asAdmin({ settings_read: true, settings: true })
  })
  it('shows the new update time after saving the CNY rate', async () => {
    api.getReferralSettings.mockResolvedValue(settings)
    api.saveReferralSettings.mockResolvedValue({
      ...settings,
      withdraw_cny_rate: '7.2',
      withdraw_cny_rate_updated_at: 1_700_000_500,
    })
    const user = userEvent.setup()
    render(<SettingsPanel canWrite />, { wrapper })
    const input = await screen.findByLabelText(
      'CNY withdrawal rate (1 USD = ? CNY)'
    )
    expect(screen.getByTestId('cny-updated')).toHaveTextContent('Not set')
    await user.type(input, '7.2')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(api.saveReferralSettings).toHaveBeenCalledWith({
      withdraw_cny_rate: '7.2',
    })
    await waitFor(() =>
      expect(screen.getByTestId('cny-updated')).not.toHaveTextContent('Not set')
    )
  })

  it('keeps the switch off until a default rate is entered', async () => {
    api.getReferralSettings.mockResolvedValue(settings)
    render(<SettingsPanel canWrite />, { wrapper })
    expect(
      await screen.findByRole('switch', { name: 'Referral program' })
    ).toHaveAttribute('aria-disabled', 'true')
  })
})

describe('referral settings action permissions', () => {
  it('opens settings with settings_read while every mutation control is read-only', async () => {
    asAdmin({ read: true, settings_read: true })
    render(<AdminReferral />, { wrapper })
    await userEvent.click(screen.getByRole('tab', { name: 'Settings' }))
    expect(await screen.findByLabelText('Freeze period (days)')).toBeDisabled()
    for (const field of screen.getAllByRole('spinbutton')) {
      expect(field).toBeDisabled()
    }
    expect(
      screen.getByRole('switch', { name: 'Referral program' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
    expect(api.getReferralSettings).toHaveBeenCalledTimes(1)
    expect(api.saveReferralSettings).not.toHaveBeenCalled()
  })

  it('does not let settings write substitute for settings_read', () => {
    asAdmin({ read: true, settings: true })
    render(<AdminReferral />, { wrapper })
    expect(
      screen.queryByRole('tab', { name: 'Settings' })
    ).not.toBeInTheDocument()
    expect(api.getReferralSettings).not.toHaveBeenCalled()
  })

  it('saves ordinary settings with both read and write grants', async () => {
    asAdmin({ read: true, settings_read: true, settings: true })
    api.saveReferralSettings.mockResolvedValue({ ...settings, freeze_days: 11 })
    render(<AdminReferral />, { wrapper })
    await userEvent.click(screen.getByRole('tab', { name: 'Settings' }))
    const field = await screen.findByLabelText('Freeze period (days)')
    expect(field).toBeEnabled()
    await userEvent.clear(field)
    await userEvent.type(field, '11')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(api.saveReferralSettings).toHaveBeenCalledWith({ freeze_days: 11 })
    )
  })

  it('keeps Root settings readable and writable', async () => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
    render(<AdminReferral />, { wrapper })
    await userEvent.click(screen.getByRole('tab', { name: 'Settings' }))
    expect(await screen.findByLabelText('Freeze period (days)')).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  })

  it('rejects direct settings form submission without write', async () => {
    render(<SettingsPanel canWrite={false} />, { wrapper })
    const field = await screen.findByLabelText('Freeze period (days)')
    const form = field.closest('form')
    if (!form) throw new Error('Referral settings form is missing')
    fireEvent.submit(form)
    await act(async () => {})
    expect(api.saveReferralSettings).not.toHaveBeenCalled()
  })

  it('rejects a dirty settings form after write permission is revoked', async () => {
    asAdmin({ read: true, settings_read: true, settings: true })
    render(<AdminReferral />, { wrapper })
    await userEvent.click(screen.getByRole('tab', { name: 'Settings' }))
    const field = await screen.findByLabelText('Freeze period (days)')
    await userEvent.clear(field)
    await userEvent.type(field, '11')
    act(() => asAdmin({ read: true, settings_read: true }))
    expect(field).toBeDisabled()
    const form = field.closest('form')
    if (!form) throw new Error('Referral settings form is missing')
    fireEvent.submit(form)
    await act(async () => {})
    expect(api.saveReferralSettings).not.toHaveBeenCalled()
  })
})

describe('queued referral settings authority', () => {
  it('rejects queued settings writes after read and write revocation unmounts the panel', async () => {
    asAdmin({ read: true, settings_read: true, settings: true })
    let resume = () => {}
    const gate = new Promise<void>((resolve) => {
      resume = resolve
    })
    const entered = vi.fn()
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: {
          onMutate: async () => {
            entered()
            await gate
          },
        },
      },
    })
    render(
      <QueryClientProvider client={client}>
        <AdminReferral />
      </QueryClientProvider>
    )
    await userEvent.click(screen.getByRole('tab', { name: 'Settings' }))
    const field = await screen.findByLabelText('Freeze period (days)')
    fireEvent.change(field, { target: { value: '41' } })
    const form = field.closest('form')
    if (!form) throw new Error('Referral settings form is missing')
    fireEvent.submit(form)
    await waitFor(() => expect(entered).toHaveBeenCalledTimes(1))
    act(() => asAdmin({ read: true }))
    expect(
      screen.queryByLabelText('Freeze period (days)')
    ).not.toBeInTheDocument()
    await act(async () => {
      resume()
    })
    await waitFor(() => expect(client.isMutating()).toBe(0))
    expect(api.saveReferralSettings).not.toHaveBeenCalled()
  })
})

describe('buildSettingsUpdate', () => {
  it('sends an emptied money field in clear and leaves unchanged ones out', () => {
    const saved = { ...settings, default_rate: 10, min_withdraw_usd: 20 }
    const drafts = { ...toDrafts(saved), min_withdraw_usd: '' }
    expect(buildSettingsUpdate(saved, drafts, false)).toEqual({
      clear: ['min_withdraw_usd'],
    })
  })
})

describe('useAdminReferralBadge', () => {
  it('shows the pending count from the scoped sidebar badge API', async () => {
    asAdmin({ read: true })
    serveSidebarBadges({ referral_withdrawals_pending: 3 })
    const { result } = renderHook(() => useAdminReferralBadge(), { wrapper })
    await waitFor(() => expect(result.current).toBe('3'))
    expect(http.get).toHaveBeenCalledWith('/api/adty/badges')
    expect(api.getReferralBadges).not.toHaveBeenCalled()
  })

  it('does not show a badge without the read permission', () => {
    asAdmin({})
    serveSidebarBadges({ referral_withdrawals_pending: 3 })
    const { result } = renderHook(() => useAdminReferralBadge(), { wrapper })
    expect(result.current).toBeUndefined()
  })

  it('does not treat a failed badge refetch as the previous successful number', async () => {
    asAdmin({ read: true })
    serveSidebarBadges({ referral_partners: 2 })
    const { client } = renderWithReferralBadge(<div />)
    expect(await screen.findByTestId('referral-sidebar-badge')).toHaveTextContent(
      '2'
    )
    http.failNext = true
    await act(async () => {
      await client.invalidateQueries({ queryKey: ['adty-sidebar-badges'] })
    })
    await waitFor(() =>
      expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent(
        'Sidebar badge unavailable'
      )
    )
    expect(screen.getByTestId('referral-sidebar-badge')).not.toHaveTextContent(
      '2'
    )
  })
})

describe('official referral sidebar badge refresh', () => {
  it('updates immediately after a partner review without waiting 5 minutes', async () => {
    asAdmin({ read: true, review: true })
    serveSidebarBadges({ referral_partners: 2 })
    api.getAffiliates.mockResolvedValue({
      page: 1,
      page_size: 20,
      total: 1,
      items: [affiliate()],
    })
    api.reviewPartner.mockImplementation(async () => {
      serveSidebarBadges({ referral_partners: 1 })
    })
    const user = userEvent.setup()
    const initialGets = badgeGets()
    renderWithReferralBadge(
      <AffiliatePanel canReview canAdjust={false} partnerOnly />
    )
    expect(await screen.findByTestId('referral-sidebar-badge')).toHaveTextContent(
      '2'
    )
    await user.click(await screen.findByRole('button', { name: 'Review' }))
    await user.type(screen.getByLabelText('Partner rate (%)'), '16')
    const afterLoad = badgeGets()
    expect(afterLoad).toBeGreaterThan(initialGets)
    await user.click(screen.getByRole('button', { name: 'Approve' }))
    await waitFor(() =>
      expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent(
        '1'
      )
    )
    expect(api.reviewPartner).toHaveBeenCalled()
    expect(badgeGets()).toBeGreaterThan(afterLoad)
  })

  it('updates immediately after a withdrawal review without waiting 5 minutes', async () => {
    asAdmin({ read: true, review: true, payout: true })
    serveSidebarBadges({ referral_withdrawals_pending: 2 })
    api.reviewWithdrawal.mockImplementation(async () => {
      serveSidebarBadges({ referral_withdrawals_pending: 1 })
    })
    const user = userEvent.setup()
    renderWithReferralBadge(<WithdrawalPanel canReview canPayout />)
    expect(await screen.findByTestId('referral-sidebar-badge')).toHaveTextContent(
      '2'
    )
    const afterLoad = badgeGets()
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    await waitFor(() =>
      expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent(
        '1'
      )
    )
    expect(api.reviewWithdrawal).toHaveBeenCalled()
    expect(badgeGets()).toBeGreaterThan(afterLoad)
  })

  it('updates immediately after marking a withdrawal paid without waiting 5 minutes', async () => {
    asAdmin({ read: true, review: true, payout: true })
    serveSidebarBadges({ referral_withdrawals_approved: 1 })
    api.payWithdrawal.mockImplementation(async () => {
      serveSidebarBadges({})
    })
    const user = userEvent.setup()
    renderWithReferralBadge(<WithdrawalPanel canReview canPayout />)
    expect(await screen.findByTestId('referral-sidebar-badge')).toHaveTextContent(
      '1'
    )
    const afterLoad = badgeGets()
    await user.click(await screen.findByRole('button', { name: 'Mark as paid' }))
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent('')
    )
    expect(api.payWithdrawal).toHaveBeenCalled()
    expect(badgeGets()).toBeGreaterThan(afterLoad)
  })

  it('updates immediately after a settlement without waiting 5 minutes', async () => {
    asAdmin({ read: true, adjust: true })
    serveSidebarBadges({ referral_commissions_frozen: 4 })
    api.runSettlement.mockImplementation(async () => {
      serveSidebarBadges({})
    })
    const user = userEvent.setup()
    renderWithReferralBadge(<AdminReferral />)
    expect(await screen.findByTestId('referral-sidebar-badge')).toHaveTextContent(
      '4'
    )
    const afterLoad = badgeGets()
    await user.click(screen.getByRole('button', { name: 'Run settlement now' }))
    await waitFor(() =>
      expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent('')
    )
    expect(api.runSettlement).toHaveBeenCalled()
    expect(badgeGets()).toBeGreaterThan(afterLoad)
  })

  it('does not clear the referral todo when the admin menu is opened', async () => {
    asAdmin({ read: true, review: true })
    serveSidebarBadges({ referral_partners: 3 })
    const user = userEvent.setup()
    renderWithReferralBadge(<AdminReferral />)
    expect(await screen.findByTestId('referral-sidebar-badge')).toHaveTextContent(
      '3'
    )
    await user.click(screen.getByRole('tab', { name: 'Partner applications' }))
    expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent('3')
    await user.click(screen.getByRole('tab', { name: 'Withdrawals' }))
    expect(screen.getByTestId('referral-sidebar-badge')).toHaveTextContent('3')
    expect(api.reviewPartner).not.toHaveBeenCalled()
    expect(api.reviewWithdrawal).not.toHaveBeenCalled()
    expect(api.payWithdrawal).not.toHaveBeenCalled()
  })
})
