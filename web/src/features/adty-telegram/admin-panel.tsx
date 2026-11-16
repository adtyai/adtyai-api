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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SecureVerificationDialog } from '@/features/auth/secure-verification/components/secure-verification-dialog'
import { useSecureVerification } from '@/features/auth/secure-verification/hooks/use-secure-verification'
import { api } from '@/lib/api'
import { formatTimestamp } from '@/lib/format'
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
      if (!body.success) throw new Error(body.message || t('Failed to load data'))
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
      if (!body.success) throw new Error(body.message || t('Failed to save'))
      setSecret(body.data?.webhook_secret || '')
      setToken('')
      await queryClient.invalidateQueries({ queryKey: ['adty-telegram-admin'] })
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  // A test checks the token with Telegram and sends a test notice to the bot
  // this Root account has bound; without a binding only the token is checked.
  const test = async (purpose: string) => {
    const proof = await verification.requestVerification({
      scope: 'adty.notice.configure',
      context: { action: 'telegram.test', target: purpose },
      title: t('Confirm it is you'),
      description: t('Testing a bot requires a fresh verification.'),
    })
    if (!proof?.proof_token) return
    setPending(true)
    setError('')
    try {
      const response = await api.post(
        `/api/adty/admin/telegram/bots/${purpose}/test`,
        undefined,
        { headers: { 'X-Security-Proof': proof.proof_token } }
      )
      const body = response.data as { success?: boolean; message?: string }
      if (!body.success) throw new Error(body.message || t('Bot test failed'))
      toast.success(t('Bot test passed'))
      await queryClient.invalidateQueries({ queryKey: ['adty-telegram-admin'] })
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Bot test failed')))
    } finally {
      setPending(false)
    }
  }
  const time = (value: number) => (value ? formatTimestamp(value) : t('Never'))

  return (
    <Card>
      <CardContent className='flex flex-col gap-4 py-4'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Telegram bots')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Two notification bots. Tokens stay masked. Saving is separate from a connection test.')}{' '}
            {t('A test checks the token and sends a test notice to the bot you have bound yourself.')}
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
              {t('Saved')} {time(bot.saved_at)} · {t('Tested')} {time(bot.tested_at)}
            </p>
            <div className='flex flex-wrap gap-2'>
              <Button type='button' variant='outline' disabled={pending || token.trim() === ''} onClick={() => void save(bot.purpose)}>
                {t('Save')}
              </Button>
              <Button type='button' variant='outline' disabled={pending || !bot.token_mask} onClick={() => void test(bot.purpose)}>
                {t('Send test')}
              </Button>
            </div>
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
