import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SecureVerificationDialog } from '@/features/auth/secure-verification/components/secure-verification-dialog'
import { useSecureVerification } from '@/features/auth/secure-verification/hooks/use-secure-verification'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

type BotView = {
  purpose: string
  token_mask: string
  identity_verified: boolean
  saved_at: number
  tested_at: number
  pending: number
}

export function TelegramAdminPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const verification = useSecureVerification()
  const bots = useQuery({
    queryKey: ['adty-telegram-admin'],
    queryFn: async () => {
      const response = await api.get('/api/adty/admin/telegram/bots')
      const body = response.data as { success?: boolean; data?: { bots?: BotView[] }; message?: string }
      if (!body.success) throw new Error(body.message || 'Failed to load data')
      return body.data?.bots ?? []
    },
  })
  const [token, setToken] = useState('')
  const [secret, setSecret] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const save = async (purpose: string) => {
    const proof = await verification.requestVerification({
      scope: 'adty.notice.configure',
      context: { action: 'telegram.save', target: purpose },
      title: t('Confirm it is you'),
      description: t('Changing a bot token requires a fresh verification.'),
    })
    if (!proof?.proof_token) return
    setPending(true)
    setError('')
    try {
      const response = await api.put(
        `/api/adty/admin/telegram/bots/${purpose}`,
        { token, enabled: true },
        { headers: { 'X-Security-Proof': proof.proof_token } }
      )
      const body = response.data as { success?: boolean; message?: string; data?: { webhook_secret?: string } }
      if (!body.success) throw new Error(body.message || 'Failed to save')
      setSecret(body.data?.webhook_secret || '')
      setToken('')
      await queryClient.invalidateQueries({ queryKey: ['adty-telegram-admin'] })
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <CardContent className='flex flex-col gap-4 py-4'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Telegram bots')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Two notification bots. Tokens stay masked. Saving is separate from a connection test.')}
          </p>
        </div>
        {(bots.data ?? []).map((bot) => (
          <div key={bot.purpose} className='flex flex-col gap-2'>
            <p className='text-sm'>
              {bot.purpose === 'user' ? t('User bot') : t('Admin bot')}
              <span className='text-muted-foreground'> · {bot.token_mask || t('Not configured')}</span>
            </p>
            <p className='text-muted-foreground text-sm tabular-nums'>
              {bot.identity_verified ? t('Identity verified') : t('Identity not verified')} · {t('Pending retries')} {bot.pending}
            </p>
            <p className='text-muted-foreground text-sm tabular-nums'>
              {t('Saved')} {bot.saved_at || 0} · {t('Tested')} {bot.tested_at || 0}
            </p>
            <Button type='button' variant='outline' disabled={pending || token.trim() === ''} onClick={() => void save(bot.purpose)}>
              {t('Save')}
            </Button>
          </div>
        ))}
        <div className='flex flex-col gap-1'>
          <Label htmlFor='bot-token'>{t('Bot token')}</Label>
          <Input id='bot-token' type='password' autoComplete='off' value={token} onChange={(event) => setToken(event.target.value)} />
        </div>
        {secret && <p className='text-sm break-all'>{t('Webhook secret')}: {secret}</p>}
        {error && <p className='text-destructive text-sm'>{error}</p>}
        <SecureVerificationDialog {...verification.dialogProps} />
      </CardContent>
    </Card>
  )
}
