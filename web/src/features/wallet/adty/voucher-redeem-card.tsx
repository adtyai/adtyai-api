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
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

export function VoucherRedeemCard(props: { onRedeemed: () => void }) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [tradeNo, setTradeNo] = useState('')
  const [pending, setPending] = useState(false)

  const submit = async () => {
    setPending(true)
    setError('')
    try {
      const response = await api.post('/api/adty/vouchers/redeem', {
        code: code.trim(),
      })
      const body = response.data as {
        success?: boolean
        message?: string
        data?: { trade_no?: string }
      }
      if (!body.success) {
        setError(t(body.message || 'Voucher code is invalid'))
        return
      }
      setTradeNo(body.data?.trade_no ?? '')
      setCode('')
      props.onRedeemed()
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Voucher code is invalid')))
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <CardContent className='flex flex-col gap-3 py-4'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Redeem a code')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('A recharge code adds principal once. It does not add a bonus.')}
          </p>
        </div>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
          <div className='flex flex-1 flex-col gap-1'>
            <Label htmlFor='voucher-code'>{t('Recharge code')}</Label>
            <Input
              id='voucher-code'
              value={code}
              autoComplete='off'
              onChange={(event) => setCode(event.target.value)}
            />
          </div>
          <Button type='button' disabled={pending || code.trim() === ''} onClick={() => void submit()}>
            {t('Redeem')}
          </Button>
        </div>
        {error && <p className='text-destructive text-sm'>{error}</p>}
        {tradeNo && (
          <p className='text-sm'>
            {t('Redeemed as order {{trade}}', { trade: tradeNo })}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
