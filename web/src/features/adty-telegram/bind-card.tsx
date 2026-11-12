import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

type BindView = {
  status: string
  telegram_user_id: number
  purpose: string
}

export function TelegramBindCard(props: { purpose: 'user' | 'admin' }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const base = props.purpose === 'admin' ? '/api/adty/site/telegram' : '/api/adty/telegram'
  const status = useQuery({
    queryKey: ['adty-telegram', props.purpose],
    queryFn: async () => {
      const response = await api.get(props.purpose === 'admin' ? base : `${base}/me`)
      const body = response.data as { success?: boolean; data?: BindView; message?: string }
      if (!body.success || !body.data) throw new Error(body.message || 'Failed to load data')
      return body.data
    },
  })
  const [code, setCode] = useState('')
  const [telegramId, setTelegramId] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const start = async () => {
    setPending(true)
    setError('')
    try {
      const response = await api.post(`${base}/bind/start`)
      const body = response.data as { success?: boolean; message?: string; data?: { code?: string } }
      if (!body.success || !body.data?.code) throw new Error(body.message || 'Failed to save')
      setCode(body.data.code)
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  const confirm = async () => {
    setPending(true)
    setError('')
    try {
      const response = await api.post(`${base}/bind/confirm`, {
        purpose: props.purpose,
        code,
        telegram_user_id: Number(telegramId),
      })
      const body = response.data as { success?: boolean; message?: string }
      if (!body.success) throw new Error(body.message || 'Failed to save')
      setCode('')
      setTelegramId('')
      await queryClient.invalidateQueries({ queryKey: ['adty-telegram', props.purpose] })
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  const bound = status.data?.status === 'active'
  return (
    <Card>
      <CardContent className='flex flex-col gap-4 py-4'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Telegram notices')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('This bot only sends notifications. Account changes stay on the website.')}
          </p>
        </div>
        {status.isLoading && <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>}
        {status.isError && (
          <Button type='button' variant='outline' onClick={() => void status.refetch()}>
            {t('Retry')}
          </Button>
        )}
        {status.data && (
          <p className='text-sm'>
            <span className='bg-muted mr-2 inline-block size-2 rounded-full align-middle' />
            {bound ? t('Bound') : status.data.status === 'needs_reconfirm' ? t('Needs confirmation') : t('Not bound')}
            {status.data.telegram_user_id > 0 && (
              <span className='text-muted-foreground tabular-nums'> · {status.data.telegram_user_id}</span>
            )}
          </p>
        )}
        {!bound && (
          <div className='flex flex-col gap-3'>
            <Button type='button' variant='outline' disabled={pending} onClick={() => void start()}>
              {t('Start binding')}
            </Button>
            {code && <p className='text-sm break-all'>{t('Binding code')}: {code}</p>}
            <div className='flex flex-col gap-1'>
              <Label htmlFor={`tg-id-${props.purpose}`}>{t('Telegram user id')}</Label>
              <Input id={`tg-id-${props.purpose}`} value={telegramId} inputMode='numeric' onChange={(event) => setTelegramId(event.target.value)} />
            </div>
            <Button type='button' disabled={pending || code === '' || telegramId.trim() === ''} onClick={() => void confirm()}>
              {t('Confirm binding')}
            </Button>
          </div>
        )}
        {error && <p className='text-destructive text-sm'>{error}</p>}
      </CardContent>
    </Card>
  )
}
