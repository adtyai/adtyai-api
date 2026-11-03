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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { handleServerError } from '@/lib/handle-server-error'

import {
  saveEpayChannel,
  type EpayChannel,
  type EpayChannelInput,
  type EpayMethodConfig,
} from '../api'

const METHODS: Array<{ id: EpayMethodConfig['method']; label: string }> = [
  { id: 'alipay', label: 'Alipay' },
  { id: 'wxpay', label: 'WeChat Pay' },
]

function emptyMethod(method: EpayMethodConfig['method']): EpayMethodConfig {
  return {
    method,
    type_code: method,
    min_cny: '0',
    max_cny: '0',
    daily_limit_cny: '0',
    enabled: false,
  }
}

function toInput(channel: EpayChannel | null): EpayChannelInput {
  const methods = METHODS.map(
    (m) => channel?.methods.find((x) => x.method === m.id) ?? emptyMethod(m.id)
  )
  return {
    name: channel?.name ?? '',
    gateway_url: channel?.gateway_url ?? '',
    merchant_id: channel?.merchant_id ?? '',
    merchant_key: '',
    priority: channel?.priority ?? 0,
    weight: channel?.weight ?? 1,
    order_mode: channel?.order_mode ?? 'api',
    probe_mode: channel?.probe_mode ?? 'off',
    unpaid_threshold: channel?.unpaid_threshold ?? 0,
    methods: methods.map((m) => ({ ...m, type_code: m.type_code || m.method })),
  }
}

type ChannelDrawerProps = {
  open: boolean
  channel: EpayChannel | null
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}

/** Add or edit an epay channel (root only). The key field is empty when editing; empty keeps it. */
export function ChannelDrawer(props: ChannelDrawerProps) {
  const { t } = useTranslation()
  const [input, setInput] = useState<EpayChannelInput>(() =>
    toInput(props.channel)
  )
  useEffect(() => {
    if (props.open) setInput(toInput(props.channel))
  }, [props.open, props.channel])

  const save = useMutation({
    mutationFn: () => saveEpayChannel(props.channel?.id ?? null, input),
    onSuccess: () => {
      toast.success(t('Payment channel saved'))
      props.onSaved()
      props.onOpenChange(false)
    },
    onError: (error) => handleServerError(error),
  })

  const set = <K extends keyof EpayChannelInput>(
    key: K,
    value: EpayChannelInput[K]
  ) => setInput((prev) => ({ ...prev, [key]: value }))
  const setMethod = (index: number, patch: Partial<EpayMethodConfig>) =>
    setInput((prev) => ({
      ...prev,
      methods: prev.methods.map((m, i) =>
        i === index ? { ...m, ...patch } : m
      ),
    }))
  const anyMethod = input.methods.some((m) => m.enabled)
  const needsKey = !props.channel && input.merchant_key.trim() === ''

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName()}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {props.channel
              ? t('Edit payment channel')
              : t('Add payment channel')}
          </SheetTitle>
          <SheetDescription>
            {t('Only the super administrator can change channels and keys.')}
          </SheetDescription>
        </SheetHeader>
        <form
          className={sideDrawerFormClassName()}
          onSubmit={(event) => {
            event.preventDefault()
            save.mutate()
          }}
        >
          <div className='space-y-2'>
            <Label htmlFor='epay-name'>{t('Name')}</Label>
            <Input
              id='epay-name'
              value={input.name}
              onChange={(e) => set('name', e.target.value)}
            />
          </div>
          <div className='space-y-2'>
            <Label htmlFor='epay-url'>{t('Gateway address')}</Label>
            <Input
              id='epay-url'
              value={input.gateway_url}
              placeholder='https://pay.example.com'
              onChange={(e) => set('gateway_url', e.target.value)}
            />
          </div>
          <div className='grid grid-cols-2 gap-3'>
            <div className='space-y-2'>
              <Label htmlFor='epay-pid'>{t('Merchant ID')}</Label>
              <Input
                id='epay-pid'
                value={input.merchant_id}
                onChange={(e) => set('merchant_id', e.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='epay-key'>{t('Merchant key')}</Label>
              <Input
                id='epay-key'
                type='password'
                autoComplete='new-password'
                value={input.merchant_key}
                placeholder={
                  props.channel ? t('Leave empty to keep the current key') : ''
                }
                onChange={(e) => set('merchant_key', e.target.value)}
              />
            </div>
          </div>
          <div className='grid grid-cols-3 gap-3'>
            <div className='space-y-2'>
              <Label htmlFor='epay-priority'>{t('Priority')}</Label>
              <Input
                id='epay-priority'
                type='number'
                value={input.priority}
                onChange={(e) => set('priority', Number(e.target.value) || 0)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='epay-weight'>{t('Weight')}</Label>
              <Input
                id='epay-weight'
                type='number'
                min={1}
                value={input.weight}
                onChange={(e) => set('weight', Number(e.target.value) || 1)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='epay-threshold'>{t('Unpaid limit')}</Label>
              <Input
                id='epay-threshold'
                type='number'
                min={0}
                value={input.unpaid_threshold}
                onChange={(e) =>
                  set('unpaid_threshold', Number(e.target.value) || 0)
                }
              />
            </div>
          </div>
          <div className='grid grid-cols-2 gap-3'>
            <div className='space-y-2'>
              <Label htmlFor='epay-mode'>{t('Order mode')}</Label>
              <NativeSelect
                id='epay-mode'
                value={input.order_mode}
                onChange={(e) =>
                  set(
                    'order_mode',
                    e.target.value as EpayChannelInput['order_mode']
                  )
                }
              >
                <option value='api'>{t('API (mapi.php)')}</option>
                <option value='submit'>
                  {t('Page redirect (submit.php)')}
                </option>
              </NativeSelect>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='epay-probe'>{t('Health check')}</Label>
              <NativeSelect
                id='epay-probe'
                value={input.probe_mode}
                onChange={(e) => set('probe_mode', e.target.value)}
              >
                <option value='off'>{t('Off')}</option>
                <option value='query'>act=query</option>
                <option value='balance'>act=balance</option>
                <option value='order'>{t('Query a missing order')}</option>
              </NativeSelect>
            </div>
          </div>
          <fieldset className='space-y-3'>
            <legend className='text-sm font-medium'>
              {t('Payment methods')}
            </legend>
            {input.methods.map((m, index) => (
              <div key={m.method} className='space-y-2 rounded-lg border p-3'>
                <label className='flex items-center gap-2 text-sm font-medium'>
                  <Checkbox
                    checked={m.enabled}
                    onCheckedChange={(checked) =>
                      setMethod(index, { enabled: checked === true })
                    }
                  />
                  {t(METHODS[index].label)}
                </label>
                {m.enabled && (
                  <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
                    <div className='space-y-1'>
                      <Label htmlFor={`type-${m.method}`} className='text-xs'>
                        {t('Type code')}
                      </Label>
                      <Input
                        id={`type-${m.method}`}
                        value={m.type_code}
                        onChange={(e) =>
                          setMethod(index, { type_code: e.target.value })
                        }
                      />
                    </div>
                    <div className='space-y-1'>
                      <Label htmlFor={`min-${m.method}`} className='text-xs'>
                        {t('Min ¥')}
                      </Label>
                      <Input
                        id={`min-${m.method}`}
                        type='number'
                        min={0}
                        value={m.min_cny}
                        onChange={(e) =>
                          setMethod(index, { min_cny: e.target.value })
                        }
                      />
                    </div>
                    <div className='space-y-1'>
                      <Label htmlFor={`max-${m.method}`} className='text-xs'>
                        {t('Max ¥')}
                      </Label>
                      <Input
                        id={`max-${m.method}`}
                        type='number'
                        min={0}
                        value={m.max_cny}
                        onChange={(e) =>
                          setMethod(index, { max_cny: e.target.value })
                        }
                      />
                    </div>
                    <div className='space-y-1'>
                      <Label htmlFor={`daily-${m.method}`} className='text-xs'>
                        {t('Daily limit ¥')}
                      </Label>
                      <Input
                        id={`daily-${m.method}`}
                        type='number'
                        min={0}
                        value={m.daily_limit_cny}
                        onChange={(e) =>
                          setMethod(index, { daily_limit_cny: e.target.value })
                        }
                      />
                    </div>
                  </div>
                )}
              </div>
            ))}
            {!anyMethod && (
              <p className='text-destructive text-xs'>
                {t('Enable at least one payment method')}
              </p>
            )}
          </fieldset>
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
              disabled={!anyMethod || needsKey || save.isPending}
            >
              {t('Save')}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  )
}
