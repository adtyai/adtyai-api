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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createReferralWithdrawal,
  uploadReferralAsset,
  type RateChange,
} from '../api'
import {
  ADDRESS_HINTS,
  isValidUsdtAddress,
  USDT_NETWORKS,
  usdtNetAmount,
  type UsdtNetwork,
} from '../lib/address'
import {
  newIdempotencyKey,
  withdrawTargetAvailable,
  WITHDRAWAL_TARGET_LABELS,
} from '../lib/labels'
import type { ReferralProfile, WithdrawalAccountType } from '../types'

const TARGETS: WithdrawalAccountType[] = ['balance', 'usdt', 'alipay', 'wechat']

type WithdrawDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  profile: ReferralProfile
  onDone: () => void
}

export function WithdrawDrawer(props: WithdrawDrawerProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const profile = props.profile
  const [target, setTarget] = useState<WithdrawalAccountType>('balance')
  const [amount, setAmount] = useState('')
  const [network, setNetwork] = useState<UsdtNetwork | ''>('')
  const [address, setAddress] = useState('')
  const [payee, setPayee] = useState('')
  const [qrUrl, setQrUrl] = useState('')
  const [cnyRate, setCnyRate] = useState(profile.cny_rate)
  const [confirmingUsdt, setConfirmingUsdt] = useState(false)
  const [rateChange, setRateChange] = useState<RateChange | null>(null)
  const [idempotencyKey, setIdempotencyKey] = useState(newIdempotencyKey)

  const available = profile.account.available_amount
  const value = Number(amount)
  const fee = profile.usdt_withdraw_fee ?? 0
  const minimum = profile.min_withdraw_usd ?? 0
  const isCny = target === 'alipay' || target === 'wechat'
  const cnyAmount =
    isCny && value > 0 ? (value * Number(cnyRate)).toFixed(2) : ''
  const net = usdtNetAmount(value, fee)

  const validate = (): string => {
    if (!(value > 0)) return t('Enter a withdrawal amount')
    if (value > available) return t('Not enough withdrawable commission')
    if (target !== 'balance' && value < minimum) {
      return t('The minimum withdrawal is ${{amount}}', { amount: minimum })
    }
    if (target === 'usdt') {
      if (network === '') return t('Choose a network')
      if (!isValidUsdtAddress(network, address)) {
        return t('The address does not match the {{network}} network', {
          network,
        })
      }
      if (net <= 0) return t('The amount must be larger than the network fee')
    }
    if (isCny && (qrUrl === '' || payee.trim() === '')) {
      return t('Upload the payment QR code and enter the payee name')
    }
    return ''
  }
  const error = validate()

  const reset = () => {
    setAmount('')
    setAddress('')
    setPayee('')
    setQrUrl('')
    setNetwork('')
    setIdempotencyKey(newIdempotencyKey())
  }

  const submit = useMutation({
    mutationFn: (expectedRate: string) =>
      createReferralWithdrawal({
        amount: value.toFixed(2),
        account_type: target,
        account_network: target === 'usdt' ? network : undefined,
        account_no: target === 'usdt' ? address.trim() : undefined,
        account_name: isCny ? payee.trim() : undefined,
        qr_image_url: isCny ? qrUrl : undefined,
        expected_cny_rate: isCny ? expectedRate : undefined,
        idempotency_key: idempotencyKey,
      }),
    onSuccess: (result) => {
      if (result.kind === 'rate_changed') {
        setRateChange(result)
        return
      }
      toast.success(
        target === 'balance'
          ? t('Transferred to your balance')
          : t('Withdrawal submitted')
      )
      reset()
      props.onDone()
      props.onOpenChange(false)
    },
    onError: (err) => handleServerError(err),
  })

  const upload = useMutation({
    mutationFn: (file: File) => uploadReferralAsset(file),
    onSuccess: setQrUrl,
    onError: (err) => handleServerError(err),
  })

  const onSubmit = () => {
    if (error) return
    if (target === 'usdt') {
      setConfirmingUsdt(true)
      return
    }
    submit.mutate(cnyRate)
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-lg')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Withdraw commission')}</SheetTitle>
          <SheetDescription>
            {t('Withdrawable: {{amount}}', {
              amount: formatUsd(available, locale),
            })}
          </SheetDescription>
        </SheetHeader>
        <form
          className={sideDrawerFormClassName()}
          onSubmit={(event) => {
            event.preventDefault()
            onSubmit()
          }}
        >
          <fieldset className='space-y-2'>
            <legend className='text-sm font-medium'>{t('Withdraw to')}</legend>
            <div className='grid grid-cols-2 gap-2'>
              {TARGETS.map((item) => {
                const enabled = withdrawTargetAvailable(profile, item)
                return (
                  <Button
                    key={item}
                    type='button'
                    variant='outline'
                    disabled={!enabled}
                    aria-pressed={target === item}
                    className={
                      target === item ? 'border-foreground bg-foreground/5' : ''
                    }
                    onClick={() => setTarget(item)}
                  >
                    {t(WITHDRAWAL_TARGET_LABELS[item])}
                    {!enabled && (
                      <span className='text-muted-foreground ml-1 text-xs'>
                        {t('Not available yet')}
                      </span>
                    )}
                  </Button>
                )
              })}
            </div>
          </fieldset>

          <div className='space-y-2'>
            <Label htmlFor='withdraw-amount'>{t('Amount (USD)')}</Label>
            <Input
              id='withdraw-amount'
              type='number'
              min={0}
              step='0.01'
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
            {target === 'balance' && (
              <p className='text-muted-foreground text-xs'>
                {t('Credited to your balance right away, no fee.')}
              </p>
            )}
          </div>

          {target === 'usdt' && (
            <>
              <div className='space-y-2'>
                <Label htmlFor='withdraw-network'>{t('Network')}</Label>
                <NativeSelect
                  id='withdraw-network'
                  value={network}
                  onChange={(event) => {
                    setNetwork(event.target.value as UsdtNetwork | '')
                    setAddress('')
                  }}
                >
                  <option value=''>{t('Choose a network')}</option>
                  {USDT_NETWORKS.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                </NativeSelect>
              </div>
              <div className='space-y-2'>
                <Label htmlFor='withdraw-address'>{t('USDT address')}</Label>
                <Input
                  id='withdraw-address'
                  value={address}
                  disabled={network === ''}
                  placeholder={network ? t(ADDRESS_HINTS[network]) : ''}
                  aria-invalid={
                    address !== '' &&
                    network !== '' &&
                    !isValidUsdtAddress(network, address)
                  }
                  onChange={(event) => setAddress(event.target.value)}
                />
              </div>
              <p className='text-muted-foreground text-sm'>
                {t('Network fee {{fee}} USDT, you receive {{net}} USDT', {
                  fee,
                  net: net.toFixed(2),
                })}
              </p>
            </>
          )}

          {isCny && (
            <>
              <Alert>
                <AlertDescription>
                  {t(
                    'Current rate 1 USD = {{rate}} CNY, updated {{time}}. You receive ¥{{cny}}.',
                    {
                      rate: cnyRate,
                      time: formatTimestamp(profile.cny_rate_updated_at),
                      cny: cnyAmount || '0.00',
                    }
                  )}
                </AlertDescription>
              </Alert>
              <div className='space-y-2'>
                <Label htmlFor='withdraw-payee'>{t('Payee name')}</Label>
                <Input
                  id='withdraw-payee'
                  value={payee}
                  onChange={(event) => setPayee(event.target.value)}
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='withdraw-qr'>{t('Payment QR code')}</Label>
                <Input
                  id='withdraw-qr'
                  type='file'
                  accept='image/png,image/jpeg,image/webp'
                  onChange={(event) => {
                    const file = event.target.files?.[0]
                    if (file) upload.mutate(file)
                  }}
                />
                {qrUrl && (
                  <img
                    src={qrUrl}
                    alt={t('Payment QR code')}
                    className='h-40 w-40 rounded-lg border object-contain'
                  />
                )}
              </div>
            </>
          )}

          {error && amount !== '' && (
            <p className='text-destructive text-sm' role='alert'>
              {error}
            </p>
          )}
          <div className='flex justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => props.onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              disabled={Boolean(error) || submit.isPending || upload.isPending}
            >
              {t('Submit')}
            </Button>
          </div>
        </form>
      </SheetContent>

      <ConfirmDialog
        open={confirmingUsdt}
        onOpenChange={setConfirmingUsdt}
        title={t('Confirm the USDT withdrawal')}
        desc={
          <div className='space-y-2 text-sm'>
            <p>
              {network} · <span className='font-mono break-all'>{address}</span>
            </p>
            <p data-testid='usdt-net'>
              {t('You receive {{net}} USDT', { net: net.toFixed(2) })}
            </p>
            <p className='text-destructive'>
              {t('A wrong network or address cannot be recovered.')}
            </p>
          </div>
        }
        confirmText={t('Confirm')}
        isLoading={submit.isPending}
        handleConfirm={() => {
          setConfirmingUsdt(false)
          submit.mutate('')
        }}
      />
      <ConfirmDialog
        open={rateChange !== null}
        onOpenChange={(open) => !open && setRateChange(null)}
        title={t('The exchange rate has changed')}
        desc={
          rateChange ? (
            <p className='text-sm' data-testid='rate-change'>
              {t('New rate 1 USD = {{rate}} CNY, you receive ¥{{cny}}.', {
                rate: rateChange.rate,
                cny: rateChange.cnyAmount,
              })}
            </p>
          ) : (
            ''
          )
        }
        confirmText={t('Submit at the new rate')}
        isLoading={submit.isPending}
        handleConfirm={() => {
          if (!rateChange) return
          setCnyRate(rateChange.rate)
          setRateChange(null)
          submit.mutate(rateChange.rate)
        }}
      />
    </Sheet>
  )
}
