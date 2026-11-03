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
import { render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ReferralWithdrawal } from '@/features/referral/types'
import { useAuthStore } from '@/stores/auth-store'

import type { ReferralSettings } from '../api'
import { SettingsPanel } from '../components/settings-panel'
import { WithdrawalPanel } from '../components/withdrawal-panel'
import { useAdminReferralBadge } from '../hooks/use-admin-referral-badge'
import { buildSettingsUpdate, toDrafts } from '../lib/settings'

const api = vi.hoisted(() => ({
  getAdminWithdrawals: vi.fn(),
  reviewWithdrawal: vi.fn(),
  payWithdrawal: vi.fn(),
  getReferralBadges: vi.fn(),
  getReferralSettings: vi.fn(),
  saveReferralSettings: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
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

beforeEach(() => {
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
  it('shows the new update time after saving the CNY rate', async () => {
    api.getReferralSettings.mockResolvedValue(settings)
    api.saveReferralSettings.mockResolvedValue({
      ...settings,
      withdraw_cny_rate: '7.2',
      withdraw_cny_rate_updated_at: 1_700_000_500,
    })
    const user = userEvent.setup()
    render(<SettingsPanel />, { wrapper })
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
    render(<SettingsPanel />, { wrapper })
    expect(
      await screen.findByRole('switch', { name: 'Referral program' })
    ).toHaveAttribute('aria-disabled', 'true')
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
  it('shows the pending count from the badges API', async () => {
    asAdmin({ read: true })
    api.getReferralBadges.mockResolvedValue({
      partners: 1,
      pending_withdrawals: 2,
      approved_withdrawals: 1,
      total: 4,
    })
    const { result } = renderHook(() => useAdminReferralBadge(), { wrapper })
    await waitFor(() => expect(result.current).toBe('4'))
  })

  it('does not query without the read permission', () => {
    asAdmin({})
    const { result } = renderHook(() => useAdminReferralBadge(), { wrapper })
    expect(result.current).toBeUndefined()
    expect(api.getReferralBadges).not.toHaveBeenCalled()
  })
})
