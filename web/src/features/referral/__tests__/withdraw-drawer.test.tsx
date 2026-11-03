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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { WithdrawDrawer } from '../components/withdraw-drawer'
import type { ReferralProfile } from '../types'

const api = vi.hoisted(() => ({
  createReferralWithdrawal: vi.fn(),
  uploadReferralAsset: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

const TRON = 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t'

function makeProfile(
  overrides: Partial<ReferralProfile> = {}
): ReferralProfile {
  return {
    affiliate: {
      id: 1,
      user_id: 7,
      invite_code: 'ABCD2345',
      status: 'approved',
      partner_status: '',
      applicant_note: '',
      rate_override: null,
      acquisition_enabled: true,
      settlement_enabled: true,
      withdrawal_enabled: true,
      risk_reason: '',
      risk_note: '',
      site_owner_since: 0,
      created_at: 1,
    },
    account: {
      pending_amount: 0,
      available_amount: 200,
      frozen_amount: 0,
      withdrawn_amount: 0,
    },
    rate: '10',
    freeze_days: 7,
    min_withdraw_usd: 20,
    usdt_withdraw_fee: 2,
    cancel_window_minutes: 30,
    cny_rate: '7.1',
    cny_rate_updated_at: 1_700_000_000,
    enabled: true,
    site_owner: false,
    funnel: { clicks: 0, signups: 0, paying: 0, earned: '0' },
    ...overrides,
  }
}

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

function renderDrawer(profile = makeProfile()) {
  return render(
    <WithdrawDrawer
      open
      onOpenChange={() => {}}
      profile={profile}
      onDone={() => {}}
    />,
    { wrapper }
  )
}

beforeEach(() => {
  api.createReferralWithdrawal.mockReset()
  api.uploadReferralAsset.mockReset()
})

describe('WithdrawDrawer USDT', () => {
  it('keeps the address field disabled until a network is chosen', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: 'USDT' }))
    expect(screen.getByLabelText('USDT address')).toBeDisabled()
    await user.selectOptions(screen.getByLabelText('Network'), 'TRC20')
    expect(screen.getByLabelText('USDT address')).toBeEnabled()
  })

  it('shows amount minus the fee in the confirmation before submitting', async () => {
    api.createReferralWithdrawal.mockResolvedValue({
      kind: 'ok',
      withdrawal: { id: 1 },
    })
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: 'USDT' }))
    await user.type(screen.getByLabelText('Amount (USD)'), '150')
    await user.selectOptions(screen.getByLabelText('Network'), 'TRC20')
    await user.type(screen.getByLabelText('USDT address'), TRON)
    await user.click(screen.getByRole('button', { name: 'Submit' }))
    expect(screen.getByTestId('usdt-net')).toHaveTextContent('148.00')
    expect(api.createReferralWithdrawal).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() =>
      expect(api.createReferralWithdrawal).toHaveBeenCalledWith(
        expect.objectContaining({
          amount: '150.00',
          account_type: 'usdt',
          account_network: 'TRC20',
          account_no: TRON,
        })
      )
    )
  })

  it('rejects a 0x address on TRC20', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: 'USDT' }))
    await user.type(screen.getByLabelText('Amount (USD)'), '50')
    await user.selectOptions(screen.getByLabelText('Network'), 'TRC20')
    await user.type(
      screen.getByLabelText('USDT address'),
      '0x52908400098527886E0F7030069857D2E4169EE7'
    )
    expect(screen.getByRole('button', { name: 'Submit' })).toBeDisabled()
  })
})

describe('WithdrawDrawer CNY', () => {
  it('resubmits at the new rate after rate_changed is confirmed', async () => {
    api.uploadReferralAsset.mockResolvedValue('/api/referral-assets/qr.png')
    api.createReferralWithdrawal
      .mockResolvedValueOnce({
        kind: 'rate_changed',
        rate: '7.2',
        cnyAmount: '720.00',
        updatedAt: 1_700_000_100,
      })
      .mockResolvedValueOnce({ kind: 'ok', withdrawal: { id: 2 } })
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: 'Alipay' }))
    await user.type(screen.getByLabelText('Amount (USD)'), '100')
    await user.type(screen.getByLabelText('Payee name'), 'Zhang San')
    await user.upload(
      screen.getByLabelText('Payment QR code'),
      new File(['x'], 'qr.png', { type: 'image/png' })
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Submit' })).toBeEnabled()
    )
    await user.click(screen.getByRole('button', { name: 'Submit' }))
    expect(await screen.findByTestId('rate-change')).toHaveTextContent('720.00')
    expect(api.createReferralWithdrawal).toHaveBeenLastCalledWith(
      expect.objectContaining({ expected_cny_rate: '7.1' })
    )
    await user.click(
      screen.getByRole('button', { name: 'Submit at the new rate' })
    )
    await waitFor(() =>
      expect(api.createReferralWithdrawal).toHaveBeenLastCalledWith(
        expect.objectContaining({ expected_cny_rate: '7.2' })
      )
    )
  })

  it('marks Alipay and WeChat unavailable when no CNY rate is set', () => {
    renderDrawer(makeProfile({ cny_rate: '' }))
    expect(screen.getByRole('button', { name: /Alipay/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /WeChat Pay/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /balance/i })).toBeEnabled()
  })

  it('marks USDT unavailable when no fee is set', () => {
    renderDrawer(makeProfile({ usdt_withdraw_fee: null }))
    expect(screen.getByRole('button', { name: /USDT/ })).toBeDisabled()
  })
})
