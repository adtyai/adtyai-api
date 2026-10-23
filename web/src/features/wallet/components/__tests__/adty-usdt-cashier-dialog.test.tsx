import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { AdtyUsdtCashierDialog } from '../adty-usdt-cashier-dialog'

vi.mock('../../api', () => ({
  getAdtyTopupOrder: vi.fn(async () => ({ data: { status: 'pending' } })),
}))

vi.mock('@/features/chatwoot/api', () => ({
  fetchSupportContact: vi.fn(async () => ({ contact: '' })),
}))

vi.mock('@/hooks/use-copy-to-clipboard', () => ({
  useCopyToClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

const oldUnderpayCopy =
  'If the amount received is less than due, the order will not credit automatically and needs manual review. Contact support with TxID. Overpay also needs manual check.'

const creditCopy =
  'A confirmed payment credits the snapshotted USD and bonus. An unmatched transfer is not moved to manual review automatically. Contact support with the order number and TxID, and do not send more to the old address.'

function renderCashier(expireAt: number) {
  render(
    <AdtyUsdtCashierDialog
      open
      onOpenChange={() => {}}
      order={{
        order_no: 'USDT-CASH-1',
        amount_usd: 10,
        payment_method: 'usdt',
        status: 'pending',
        display_status: 'pending',
        can_continue: true,
      }}
      usdt={{
        payment_address: 'TOLDADDRESS123',
        payment_amount: '10.01',
        pay_usdt: 10.01,
        payment_url: 'https://pay.example/old',
        expire_at: expireAt,
        usdt_fx_rate: 1,
        network: 'TRC20',
      }}
    />
  )
}

describe('AdtyUsdtCashierDialog credit boundary copy', () => {
  it('does not tell users that a short payment goes to manual review', () => {
    renderCashier(Math.floor(Date.now() / 1000) + 600)
    expect(screen.queryByText(oldUnderpayCopy)).toBeNull()
    expect(screen.getByText(creditCopy)).toBeVisible()
    expect(screen.getByText('TOLDADDRESS123')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Open payment page' })).toBeVisible()
  })

  it('hides the old address and payment page after the window ends', () => {
    renderCashier(1)
    expect(screen.queryByText('TOLDADDRESS123')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Open payment page' })).toBeNull()
    expect(
      screen.getByText('Payment window closed. Do not transfer again.')
    ).toBeVisible()
    expect(screen.queryByText(oldUnderpayCopy)).toBeNull()
  })
})
