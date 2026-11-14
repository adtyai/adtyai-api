import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
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
        {config.isPending ? (
          <LoadingState />
        ) : config.isError ? (
          <ErrorState
            description={getServerErrorMessage(config.error, t('Failed to load'))}
            onRetry={() => void config.refetch()}
          />
        ) : (
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
        )}
        <SecureVerificationDialog {...verification.dialogProps} />
      </CardContent>
    </Card>
  )
}
