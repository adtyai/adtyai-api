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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import type { OwnerCustomer } from '@/features/adty-site-owner/types'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createStaffBalanceRequest,
  type StaffBalanceDirection,
} from './api'

const notice =
  'This request does not change the customer balance until the site owner or Root approves it.'

export function StaffBalanceRequestDialog(props: {
  customer: OwnerCustomer
  userId: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [direction, setDirection] = useState<StaffBalanceDirection | ''>('')
  const [amount, setAmount] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const submit = useMutation({
    retry: false,
    mutationFn: () => {
      if (direction !== 'increase' && direction !== 'decrease') {
        throw new Error(t('Invalid parameters'))
      }
      return createStaffBalanceRequest({
        customer_id: props.customer.id,
        direction,
        amount_usd: amount.trim(),
      })
    },
    onSuccess: () => {
      void client.invalidateQueries({
        queryKey: ['adty-staff', props.userId, 'balance-requests'],
      })
      props.onClose()
    },
    onError: (error) => handleServerError(error),
  })
  const ready =
    (direction === 'increase' || direction === 'decrease') &&
    amount.trim() !== '' &&
    confirmed &&
    !submit.isPending
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !submit.isPending) props.onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Request balance change')}</DialogTitle>
          <DialogDescription>{t(notice)}</DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <p className='text-sm break-all'>{props.customer.username}</p>
          <div className='flex flex-col gap-1'>
            <Label htmlFor='staff-balance-direction'>{t('Direction')}</Label>
            <NativeSelect
              id='staff-balance-direction'
              className='w-full'
              value={direction}
              onChange={(event) =>
                setDirection(event.target.value as StaffBalanceDirection | '')
              }
            >
              <NativeSelectOption value=''>{t('Direction')}</NativeSelectOption>
              <NativeSelectOption value='increase'>{t('Increase')}</NativeSelectOption>
              <NativeSelectOption value='decrease'>{t('Decrease')}</NativeSelectOption>
            </NativeSelect>
          </div>
          <div className='flex flex-col gap-1'>
            <Label htmlFor='staff-balance-amount'>{t('Amount')}</Label>
            <Input
              id='staff-balance-amount'
              value={amount}
              inputMode='decimal'
              autoComplete='off'
              onChange={(event) => setAmount(event.target.value)}
            />
          </div>
          <label className='flex items-start gap-2 text-sm'>
            <input
              type='checkbox'
              className='mt-1'
              checked={confirmed}
              onChange={(event) => setConfirmed(event.target.checked)}
            />
            <span>{t(notice)}</span>
          </label>
        </div>
        <DialogFooter>
          <Button type='button' disabled={!ready} onClick={() => submit.mutate()}>
            {t('Request balance change')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
