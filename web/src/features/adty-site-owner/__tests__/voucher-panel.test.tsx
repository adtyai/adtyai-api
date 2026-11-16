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
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { OwnerVerificationProvider } from '../components/owner-verification'
import { VoucherPanel } from '../components/voucher-panel'

const verification = vi.hoisted(() => ({ requestVerification: vi.fn() }))
vi.mock('@/features/auth/secure-verification', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useSecureVerification: () => ({
    requestVerification: verification.requestVerification,
    dialogProps: { state: { phase: 'idle' } },
    isActive: false,
    cancel: vi.fn(),
  }),
  SecureVerificationDialog: () => null,
}))

const vouchers = [
  {
    id: 11,
    code_tail: 'A1B2',
    purpose: 'main_user',
    status: 'pending',
    amount_usd: '12.5',
    quota: 6250000,
    source_kind: 'platform',
    source_site_id: 0,
    target_site_id: 0,
    issuer_id: 1,
    redeemer_id: 0,
    inherited_by_main: false,
    trade_no: '',
    created_at: 1,
    redeemed_at: 0,
  },
  {
    id: 12,
    code_tail: 'C3D4',
    purpose: 'site_customer',
    status: 'redeemed',
    amount_usd: '4',
    quota: 2000000,
    source_kind: 'earnings',
    source_site_id: 7,
    target_site_id: 0,
    issuer_id: 70,
    redeemer_id: 701,
    inherited_by_main: false,
    trade_no: 'VCH1',
    created_at: 1,
    redeemed_at: 2,
  },
]

function show(marginAllowed: boolean) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total: 1,
        items: [
          { ...vouchers[0], purpose: 'site_customer', source_kind: 'earnings' },
        ],
        margin_allowed: marginAllowed,
      },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OwnerVerificationProvider>
        <VoucherPanel siteId={7} />
      </OwnerVerificationProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  verification.requestVerification.mockResolvedValue({ proof_token: 'proof' })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('offers margin funding only when Root allows it', async () => {
  show(false)
  await screen.findByRole('table', { name: 'Issued codes' })
  expect(screen.queryByLabelText('Funded from')).not.toBeInTheDocument()
  cleanup()
  show(true)
  expect(await screen.findByLabelText('Funded from')).toBeVisible()
})

it('verifies for this site before issuing and shows the code once', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { id: 14, code: 'VCH-OWNER-CODE' } },
  })
  show(true)
  const user = userEvent.setup()
  await user.selectOptions(
    await screen.findByLabelText('Funded from'),
    'margin'
  )
  await user.type(screen.getByLabelText('Amount (USD)'), '3')
  await user.type(screen.getByLabelText('Reason'), 'gift')
  await user.click(screen.getByRole('button', { name: 'Issue code' }))
  expect(await screen.findByText('VCH-OWNER-CODE')).toBeVisible()
  expect(verification.requestVerification).toHaveBeenCalledWith(
    expect.objectContaining({
      scope: 'site.owner.manage',
      context: { action: 'voucher.issue', target: 7 },
    })
  )
  expect(post).toHaveBeenCalledWith(
    '/api/adty/site/vouchers',
    {
      purpose: 'site_customer',
      amount_usd: '3',
      source: 'margin',
      reason: 'gift',
    },
    { headers: { 'X-Security-Proof': 'proof' } }
  )
})

it('verifies against the code before cancelling it', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: {} } })
  show(false)
  const user = userEvent.setup()
  const table = await screen.findByRole('table', { name: 'Issued codes' })
  await user.click(within(table).getByRole('button', { name: 'Cancel code' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Cancel this code?',
  })
  await user.click(within(confirm).getByRole('button', { name: 'Cancel code' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/adty/site/vouchers/11/cancel',
      undefined,
      { headers: { 'X-Security-Proof': 'proof' } }
    )
  )
  expect(verification.requestVerification).toHaveBeenCalledWith(
    expect.objectContaining({
      context: { action: 'voucher.cancel', target: 11 },
    })
  )
})
