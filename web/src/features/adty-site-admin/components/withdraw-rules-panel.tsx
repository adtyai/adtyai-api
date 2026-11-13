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

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { saveAdminWithdrawRules } from '../api'

/** Root enters every withdrawal figure. The form does not suggest a minimum, fee, or rate. */
export function WithdrawRulesPanel() {
  const { t } = useTranslation()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const [minUsd, setMinUsd] = useState('')
  const [feeUsd, setFeeUsd] = useState('')
  const [alipayRate, setAlipayRate] = useState('')
  const [usdtRate, setUsdtRate] = useState('')
  const save = useMutation({
    mutationFn: () =>
      saveAdminWithdrawRules({
        min_usd: minUsd.trim(),
        fee_usd: feeUsd.trim(),
        alipay_cny_rate: alipayRate.trim(),
        usdt_usd_rate: usdtRate.trim(),
      }),
    onSuccess: () => {
      setMinUsd('')
      setFeeUsd('')
      setAlipayRate('')
      setUsdtRate('')
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  const ready = [minUsd, feeUsd, alipayRate, usdtRate].every((value) => value.trim() !== '')
  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-sm font-medium'>{t('Withdrawal rules')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t('These figures apply only to new owner withdrawal requests.')}
      </p>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='withdraw-min'>{t('Minimum amount')}</Label>
          <Input id='withdraw-min' value={minUsd} inputMode='decimal' onChange={(event) => setMinUsd(event.target.value)} />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='withdraw-fee'>{t('Fee')}</Label>
          <Input id='withdraw-fee' value={feeUsd} inputMode='decimal' onChange={(event) => setFeeUsd(event.target.value)} />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='withdraw-alipay'>{t('Alipay CNY rate')}</Label>
          <Input id='withdraw-alipay' value={alipayRate} inputMode='decimal' onChange={(event) => setAlipayRate(event.target.value)} />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='withdraw-usdt'>{t('USDT USD rate')}</Label>
          <Input id='withdraw-usdt' value={usdtRate} inputMode='decimal' onChange={(event) => setUsdtRate(event.target.value)} />
        </div>
      </div>
      <Button type='button' variant='outline' disabled={save.isPending || !ready} onClick={() => save.mutate()}>
        {t('Save')}
      </Button>
    </section>
  )
}
