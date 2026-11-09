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
import type { ActivePayment } from '../lib/active-payment'
import { EpayPayDialog } from './epay-pay-dialog'
import { HostedPayDialog } from './hosted-pay-dialog'
import { UsdtPayDialog } from './usdt-pay-dialog'

type AdtyPaymentDialogsProps = {
  payment: ActivePayment | null
  onClose: () => void
  onSettled: () => Promise<void> | void
}

/** Opens the payment dialog for the active payment. */
export function AdtyPaymentDialogs(props: AdtyPaymentDialogsProps) {
  const payment = props.payment
  if (!payment) return null
  if (payment.kind === 'hosted') {
    return (
      <HostedPayDialog
        key={payment.tradeNo ?? `${payment.method}-${payment.amount}`}
        method={payment.method}
        paymentScene={payment.paymentScene}
        amount={payment.amount}
        promoCode={payment.promoCode}
        tradeNo={payment.tradeNo}
        onClose={props.onClose}
        onSettled={props.onSettled}
      />
    )
  }
  if (payment.kind === 'usdt') {
    return (
      <UsdtPayDialog
        key={payment.tradeNo ?? `${payment.method}-${payment.amount}`}
        chain={payment.method}
        paymentScene={payment.paymentScene}
        amount={payment.amount}
        promoCode={payment.promoCode}
        tradeNo={payment.tradeNo}
        onClose={props.onClose}
        onSettled={props.onSettled}
        onChangeChain={props.onClose}
      />
    )
  }
  return (
    <EpayPayDialog
      key={payment.tradeNo ?? `${payment.method}-${payment.amount}`}
      method={payment.method}
      paymentScene={payment.paymentScene}
      amount={payment.amount}
      promoCode={payment.promoCode}
      tradeNo={payment.tradeNo}
      onClose={props.onClose}
      onSettled={props.onSettled}
    />
  )
}
