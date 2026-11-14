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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { SecureVerificationDialog } from '@/features/auth/secure-verification/components/secure-verification-dialog'
import { useSecureVerification } from '@/features/auth/secure-verification/hooks/use-secure-verification'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

type TranslationView = {
  saved_at: number
  tested_at: number
  translated_at: number
}

function ConfigGate(props: {
  isPending: boolean
  isError: boolean
  error: unknown
  onRetry: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  if (props.isPending) return <LoadingState />
  if (props.isError) {
    return (
      <ErrorState
        description={getServerErrorMessage(props.error, t('Failed to load'))}
        onRetry={props.onRetry}
      />
    )
  }
  return props.children
}

export function TranslationAdminPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const verification = useSecureVerification()
  const config = useQuery({
    queryKey: ['adty-translation'],
    queryFn: async () => {
      const response = await api.get('/api/adty/admin/translation')
      const body = response.data as { success?: boolean; data?: TranslationView; message?: string }
      if (!body.success || !body.data) throw new Error(body.message || t('Failed to load'))
      return body.data
    },
  })
  const [modelName, setModelName] = useState('translator')
  const [source, setSource] = useState('')
  const [result, setResult] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const save = async () => {
    const proof = await verification.requestVerification({
      scope: 'adty.notice.configure',
      context: { action: 'translation.save', target: 'platform' },
      title: t('Confirm it is you'),
      description: t('Changing the translator requires a fresh verification.'),
    })
    if (!proof?.proof_token) return
    setPending(true)
    setError('')
    try {
      const response = await api.put(
        '/api/adty/admin/translation',
        { mode: 'platform', model_name: modelName, enabled: true, daily_quota: 1000, retry_limit: 1 },
        { headers: { 'X-Security-Proof': proof.proof_token } }
      )
      const body = response.data as { success?: boolean; message?: string }
      if (!body.success) throw new Error(body.message || 'Failed to save')
      await queryClient.invalidateQueries({ queryKey: ['adty-translation'] })
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
          <h2 className='text-sm font-medium'>{t('System translation')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Chinese stays available when English is not ready. Customers are not charged.')}
          </p>
        </div>
        <ConfigGate
          isPending={config.isPending}
          isError={config.isError}
          error={config.error}
          onRetry={() => void config.refetch()}
        >
          <>
        {config.data && (
          <p className='text-muted-foreground text-sm tabular-nums'>
            {t('Saved')} {config.data.saved_at || 0} · {t('Tested')} {config.data.tested_at || 0} · {t('Translated')} {config.data.translated_at || 0}
          </p>
        )}
        <div className='flex flex-col gap-1'>
          <Label htmlFor='translator-model'>{t('Model')}</Label>
          <Input id='translator-model' value={modelName} onChange={(event) => setModelName(event.target.value)} />
        </div>
        <Button type='button' disabled={pending || modelName.trim() === ''} onClick={() => void save()}>
          {t('Save')}
        </Button>
        <Label htmlFor='translator-source'>{t('Chinese source')}</Label>
        <Textarea id='translator-source' value={source} onChange={(event) => setSource(event.target.value)} />
        <Button
          type='button'
          variant='outline'
          disabled={pending || source.trim() === ''}
          onClick={() => {
            setPending(true)
            void api
              .post('/api/adty/admin/translation/run', { object_key: `draft:${Date.now()}`, source })
              .then((response) => {
                const body = response.data as { success?: boolean; data?: { english?: string; notice?: string; source?: string } }
                setResult(body.data?.english || `${body.data?.source || source}\n${body.data?.notice || t('English is not ready')}`)
              })
              .catch((cause) => setError(getServerErrorMessage(cause, t('Failed to save'))))
              .finally(() => setPending(false))
          }}
        >
          {t('Translate')}
        </Button>
        {result && <p className='text-sm whitespace-pre-wrap'>{result}</p>}
        {error && <p className='text-destructive text-sm'>{error}</p>}
          </>
        </ConfigGate>
        <SecureVerificationDialog {...verification.dialogProps} />
      </CardContent>
    </Card>
  )
}
