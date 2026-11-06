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
import { afterEach, describe, expect, it, vi } from 'vitest'

import { EpayPayDialog } from '../components/epay-pay-dialog'
import { UsdtPayDialog } from '../components/usdt-pay-dialog'
import { formatCountdown } from '../lib/countdown'
import { isSafePaymentUrl } from '../lib/safe-url'
import type { AdtyOrder, AdtyOrderExt } from '../types'

const api = vi.hoisted(() => ({
  getAdtyOrder: vi.fn(),
  cancelAdtyOrder: vi.fn(),
}))
const pay = vi.hoisted(() => ({
  payWithBEpusdt: vi.fn(),
  payWithEpay: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))
vi.mock('../api-pay', () => pay)

function wrapper(props: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

const now = () => Math.floor(Date.now() / 1000)

function order(
  tradeNo: string,
  status: AdtyOrder['status'],
  ext: Partial<AdtyOrderExt>
): AdtyOrder {
  return {
    id: 1,
    user_id: 1,
    trade_no: tradeNo,
    payment_method: 'usdt.trc20',
    payment_provider: 'bepusdt',
    status,
    create_time: 0,
    complete_time: 0,
    amount: 100,
    money: 100,
    ext: {
      trade_no: tradeNo,
      provider: 'bepusdt',
      method: 'usdt.trc20',
      face_usd: '100',
      bonus_rate: '2.5',
      bonus_usd: '2.5',
      promo_bonus_usd: '0',
      credit_quota: 0,
      pay_amount: '100.01',
      pay_currency: 'USDT',
      price_snapshot: '0',
      expire_at: now() + 600,
      gateway_trade_id: 'g',
      pay_address: 'TAddr123',
      pay_url: '',
      qr_code: '',
      url_scheme: '',
      channel_id: 0,
      paid_amount: '0',
      paid_currency: '',
      paid_tx_hash: '',
      refunded_at: 0,
      ...ext,
    },
  }
}

afterEach(() => {
  vi.clearAllMocks()
  window.localStorage.clear()
})

describe('formatCountdown and isSafePaymentUrl', () => {
  it('counts down to the server expiry as mm:ss', () => {
    expect(formatCountdown(1000, 1000 - 125)).toBe('02:05')
    expect(formatCountdown(1000, 2000)).toBe('00:00')
  })
  it('only allows http(s) and the Alipay / WeChat schemes', () => {
    expect(isSafePaymentUrl('https://pay.example/x')).toBe(true)
    expect(isSafePaymentUrl('alipays://platformapi/startapp')).toBe(true)
    expect(isSafePaymentUrl('weixin://wxpay/bizpayurl')).toBe(true)
    expect(isSafePaymentUrl('javascript:alert(1)')).toBe(false)
    expect(isSafePaymentUrl('data:text/html,x')).toBe(false)
  })
})

describe('UsdtPayDialog', () => {
  it('shows the address, amount and countdown, then the credited amount', async () => {
    const pending = order('EPU1', 'pending', {})
    pay.payWithBEpusdt.mockResolvedValue(pending)
    api.getAdtyOrder
      .mockResolvedValueOnce(pending)
      .mockResolvedValue(order('EPU1', 'success', {}))
    const onSettled = vi.fn()
    render(
      <UsdtPayDialog
        chain='usdt.trc20'
        amount={100}
        onClose={() => {}}
        onSettled={onSettled}
        onChangeChain={() => {}}
      />,
      { wrapper }
    )
    expect(await screen.findByTestId('usdt-address')).toHaveTextContent(
      'TAddr123'
    )
    expect(screen.getByTestId('usdt-amount')).toHaveTextContent('100.01 USDT')
    expect(screen.getByTestId('usdt-countdown').textContent).toMatch(
      /^(09|10):\d\d$/
    )
    expect(
      await screen.findByText('Credited $102.50', {}, { timeout: 6000 })
    ).toBeVisible()
    await waitFor(() => expect(onSettled).toHaveBeenCalled())
  })

  it('changing the network cancels first and stays on the order when that fails', async () => {
    const pending = order('EPU2', 'pending', {})
    pay.payWithBEpusdt.mockResolvedValue(pending)
    api.getAdtyOrder.mockResolvedValue(pending)
    api.cancelAdtyOrder
      .mockRejectedValueOnce(new Error('transfer seen'))
      .mockResolvedValueOnce(undefined)
    const onChangeChain = vi.fn()
    const user = userEvent.setup()
    render(
      <UsdtPayDialog
        chain='usdt.trc20'
        amount={100}
        onClose={() => {}}
        onSettled={() => {}}
        onChangeChain={onChangeChain}
      />,
      { wrapper }
    )
    await screen.findByTestId('usdt-address')
    await user.click(screen.getByRole('button', { name: 'Change network' }))
    expect(
      await screen.findByText('A transfer has been detected. Please wait')
    ).toBeVisible()
    expect(onChangeChain).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Change network' }))
    await waitFor(() => expect(onChangeChain).toHaveBeenCalled())
    expect(api.cancelAdtyOrder).toHaveBeenCalledTimes(2)
  })
})

describe('EpayPayDialog', () => {
  const epayOrder = (tradeNo: string, ext: Partial<AdtyOrderExt>) => ({
    ...order(tradeNo, 'pending', {
      pay_amount: '730',
      channel_id: 3,
      provider: 'adty_epay',
      method: 'alipay',
      ...ext,
    }),
    payment_method: 'alipay',
    payment_provider: 'adty_epay',
  })

  it('shows the gateway QR code on desktop', async () => {
    const pending = epayOrder('EPY1', { qr_code: 'https://qr.alipay.com/abc' })
    pay.payWithEpay.mockResolvedValue(pending)
    api.getAdtyOrder.mockResolvedValue(pending)
    render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='pc'
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    expect(await screen.findByTestId('epay-qr')).toHaveAttribute(
      'data-value',
      'https://qr.alipay.com/abc'
    )
    expect(screen.getByTestId('epay-amount')).toHaveTextContent('¥730.00')
    expect(screen.getByText('You get $102.50')).toBeVisible()
  })

  it('adds the promo bonus to the amount you get and the credited notice', async () => {
    const pending = epayOrder('EPY-PROMO', { promo_bonus_usd: '10' })
    const paid = {
      ...pending,
      status: 'success' as const,
    }
    pay.payWithEpay.mockResolvedValue(pending)
    api.getAdtyOrder
      .mockResolvedValueOnce(pending)
      .mockResolvedValue(paid)
    render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='pc'
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    expect(await screen.findByText('You get $112.50')).toBeVisible()
    expect(
      await screen.findByText('Credited $112.50', {}, { timeout: 6000 })
    ).toBeVisible()
  })

  it('turns a payment link into a QR code when there is no QR code', async () => {
    const pending = epayOrder('EPY2', {
      pay_url: 'https://pay.example/cashier',
    })
    pay.payWithEpay.mockResolvedValue(pending)
    api.getAdtyOrder.mockResolvedValue(pending)
    render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='pc'
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    expect(await screen.findByTestId('epay-qr')).toHaveAttribute(
      'data-value',
      'https://pay.example/cashier'
    )
  })

  it('jumps to the app on phones and refuses a javascript: link', async () => {
    const navigate = vi.fn()
    const safe = epayOrder('EPY3', {
      url_scheme: 'alipays://platformapi/startapp?x=1',
    })
    pay.payWithEpay.mockResolvedValueOnce(safe)
    api.getAdtyOrder.mockResolvedValue(safe)
    const { unmount } = render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='mobile'
        navigate={navigate}
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith(
        'alipays://platformapi/startapp?x=1'
      )
    )
    unmount()

    navigate.mockReset()
    const evil = epayOrder('EPY4', { pay_url: 'javascript:alert(1)' })
    pay.payWithEpay.mockResolvedValueOnce(evil)
    api.getAdtyOrder.mockResolvedValue(evil)
    render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='mobile'
        navigate={navigate}
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    expect(
      await screen.findByText(
        'The payment link is not safe to open. Try another channel.'
      )
    ).toBeVisible()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('does not render a javascript QR code on desktop', async () => {
    const evil = epayOrder('EPY7', { qr_code: 'javascript:alert(1)' })
    pay.payWithEpay.mockResolvedValue(evil)
    api.getAdtyOrder.mockResolvedValue(evil)
    render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='pc'
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    expect(
      await screen.findByText(
        'The payment link is not safe to open. Try another channel.'
      )
    ).toBeVisible()
    expect(screen.queryByTestId('epay-qr')).not.toBeInTheDocument()
  })

  it('trying another channel cancels and reorders without that channel', async () => {
    const first = epayOrder('EPY5', { qr_code: 'https://qr/1', channel_id: 3 })
    const second = epayOrder('EPY6', { qr_code: 'https://qr/2', channel_id: 4 })
    pay.payWithEpay.mockResolvedValueOnce(first).mockResolvedValueOnce(second)
    api.getAdtyOrder.mockImplementation(async (tradeNo: string) =>
      tradeNo === 'EPY5' ? first : second
    )
    api.cancelAdtyOrder.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(
      <EpayPayDialog
        method='alipay'
        amount={100}
        device='pc'
        onClose={() => {}}
        onSettled={() => {}}
      />,
      { wrapper }
    )
    await screen.findByTestId('epay-qr')
    await user.click(
      screen.getByRole('button', { name: 'Try another channel' })
    )
    await waitFor(() => expect(pay.payWithEpay).toHaveBeenCalledTimes(2))
    expect(api.cancelAdtyOrder).toHaveBeenCalledWith('EPY5')
    expect(pay.payWithEpay.mock.calls[1][3]).toEqual([3])
    await waitFor(() =>
      expect(screen.getByTestId('epay-qr')).toHaveAttribute(
        'data-value',
        'https://qr/2'
      )
    )
  })
})
