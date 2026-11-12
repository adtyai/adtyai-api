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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

export function VoucherPanel() {
  const { t } = useTranslation()
  const [purpose, setPurpose] = useState('site_customer')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const submit = async () => {
    setPending(true)
    setError('')
    setCode('')
    try {
      const response = await api.post('/api/adty/site/vouchers', {
        purpose,
        amount_usd: amount.trim(),
        source: 'earnings',
        reason: reason.trim(),
      })
      const body = response.data as {
        success?: boolean
        message?: string
        data?: { code?: string }
      }
      if (!body.success || !body.data?.code) {
        setError(t(body.message || 'Failed to save'))
        return
      }
      setCode(body.data.code)
      setAmount('')
      setReason('')
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  return (
    <form
      className='flex max-w-xl flex-col gap-3'
      onSubmit={(event) => {
        event.preventDefault()
        void submit()
      }}
    >
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('Recharge codes')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Issuing a code freezes the same amount from available earnings. The full code is shown once.'
          )}
        </p>
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='voucher-purpose'>{t('Purpose')}</Label>
        <NativeSelect
          id='voucher-purpose'
          value={purpose}
          onChange={(event) => setPurpose(event.target.value)}
        >
          <option value='site_customer'>{t('Customer principal')}</option>
          <option value='site_owner'>{t('My spending principal')}</option>
        </NativeSelect>
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='voucher-amount'>{t('Amount (USD)')}</Label>
        <Input
          id='voucher-amount'
          inputMode='decimal'
          value={amount}
          onChange={(event) => setAmount(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='voucher-reason'>{t('Reason')}</Label>
        <Input
          id='voucher-reason'
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
      </div>
      <Button type='submit' disabled={pending || amount.trim() === '' || reason.trim() === ''}>
        {t('Issue code')}
      </Button>
      {error && <p className='text-destructive text-sm'>{error}</p>}
      {code && (
        <p className='text-sm break-all'>
          {t('Code')}: {code}
        </p>
      )}
    </form>
  )
}
