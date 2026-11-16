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
import { KeyRound, Loader2, ShieldCheck } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AuthOperationError } from '@/lib/secure-verification'

import type { PasskeyDomains } from '../../passkey/assertion'
import { PasskeyDomainSelector } from '../../passkey/components/passkey-domain-selector'
import { requestSecurityEmailCode } from '../api'
import type {
  SecureVerificationState,
  VerificationInput,
  VerificationMethod,
} from '../types'

interface SecureVerificationDialogProps {
  state: SecureVerificationState
  passkeyDomains?: PasskeyDomains | null
  onVerify: (input?: VerificationInput) => void | Promise<void>
  onCancel: () => void
  onRetry: () => void
  onInputChange: (input: VerificationInput) => void
}

const methodLabels: Record<VerificationMethod, string> = {
  '2fa': 'Authenticator code',
  passkey: 'Passkey',
  password: 'Password',
  email_code: 'Email verification code',
  session: 'Login session',
  root_exempt: 'Root exemption',
}

export function SecureVerificationDialog(props: SecureVerificationDialogProps) {
  const { t } = useTranslation()
  const inputId = useId()
  const state = props.state
  if (state.phase === 'idle') return null
  const ready =
    state.phase === 'ready' || state.phase === 'verifying' ? state : null
  const input = ready?.input
  const verifying = state.phase === 'verifying'
  const login = state.request.scope === 'auth.login'
  const acceptsBackupCode =
    state.request.scope !== '2fa.backup_codes.regenerate'
  let canVerify = state.phase === 'ready' && Boolean(input)
  if (input?.method === 'password') {
    canVerify = canVerify && input.password.length > 0
  }
  if (input?.method === '2fa' || input?.method === 'email_code') {
    canVerify = canVerify && input.code.trim().length >= 6
  }
  const showSubmit = state.phase !== 'error'
  const error = 'error' in state ? state.error : undefined
  const formId = `${inputId}-form`

  const selectMethod = (method: string) => {
    switch (method) {
      case '2fa':
        props.onInputChange({ method, code: '' })
        break
      case 'password':
        props.onInputChange({ method, password: '' })
        break
      case 'email_code':
        props.onInputChange({ method, code: '' })
        break
      case 'passkey':
        props.onInputChange({ method })
        break
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onCancel()
      }}
      title={
        <>
          <ShieldCheck className='size-5' />
          {state.request.title ??
            (login ? t('Complete sign-in') : t('Security verification'))}
        </>
      }
      description={
        state.request.description ??
        (login
          ? t('Verify your identity to finish signing in.')
          : t('Confirm your identity before accessing this sensitive action.'))
      }
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      titleClassName='flex items-center gap-2'
      footer={
        <>
          <Button type='button' variant='outline' onClick={props.onCancel}>
            {t('Cancel')}
          </Button>
          {state.phase === 'error' && (
            <Button type='button' onClick={props.onRetry}>
              {t('Retry')}
            </Button>
          )}
          {showSubmit && (
            <Button
              variant='brand'
              type='submit'
              form={formId}
              disabled={!canVerify}
            >
              {verifying && <Loader2 className='size-4 animate-spin' />}
              {t('Verify')}
            </Button>
          )}
        </>
      }
    >
      {state.phase === 'loading' && (
        <div role='status' className='flex items-center gap-2 py-4'>
          <Loader2 className='size-4 animate-spin' />
          {t('Loading verification methods...')}
        </div>
      )}
      {error && (
        <p role='alert' className='text-destructive text-sm'>
          {t(error)}
        </p>
      )}
      {ready && (
        <form
          id={formId}
          onSubmit={(event) => {
            event.preventDefault()
            if (canVerify) void props.onVerify()
          }}
          className='space-y-4 py-2'
        >
          {!input ? (
            <div className='space-y-2 text-sm'>
              <p>{t('No verification method is available for this action.')}</p>
              {ready.requirements.methods.map(
                (option) =>
                  option.reason && (
                    <p className='text-muted-foreground' key={option.method}>
                      {t(option.reason)}
                    </p>
                  )
              )}
            </div>
          ) : (
            <Tabs value={input.method} onValueChange={selectMethod}>
              <TabsList>
                {ready.requirements.methods.map((option) => (
                  <TabsTrigger
                    key={option.method}
                    value={option.method}
                    disabled={!option.available || verifying}
                  >
                    {t(methodLabels[option.method])}
                  </TabsTrigger>
                ))}
              </TabsList>
              <TabsContent value='password' className='space-y-2'>
                <Label htmlFor={inputId}>{t('Password')}</Label>
                <Input
                  id={inputId}
                  type='password'
                  autoComplete='current-password'
                  autoFocus
                  disabled={verifying}
                  value={input.method === 'password' ? input.password : ''}
                  onChange={(event) =>
                    props.onInputChange({
                      method: 'password',
                      password: event.target.value,
                    })
                  }
                />
              </TabsContent>
              <TabsContent value='email_code' className='space-y-2'>
                <EmailCodeFields
                  inputId={inputId}
                  code={input.method === 'email_code' ? input.code : ''}
                  scope={
                    state.request.scope === 'auth.login'
                      ? undefined
                      : state.request.scope
                  }
                  disabled={verifying}
                  onChange={(code) =>
                    props.onInputChange({ method: 'email_code', code })
                  }
                />
              </TabsContent>
              <TabsContent value='2fa' className='space-y-2'>
                <Label htmlFor={inputId}>
                  {acceptsBackupCode
                    ? t('Authenticator code or backup code')
                    : t('Authenticator code')}
                </Label>
                <Input
                  id={inputId}
                  autoComplete='one-time-code'
                  maxLength={acceptsBackupCode ? 9 : 6}
                  autoFocus
                  disabled={verifying}
                  value={input.method === '2fa' ? input.code : ''}
                  onChange={(event) =>
                    props.onInputChange({
                      method: '2fa',
                      code: event.target.value,
                    })
                  }
                />
                <p className='text-muted-foreground text-sm'>
                  {acceptsBackupCode
                    ? t(
                        'Enter the 6-digit authenticator code or an unused backup code.'
                      )
                    : t('Enter the 6-digit authenticator code.')}
                </p>
              </TabsContent>
              <TabsContent value='passkey' className='space-y-3'>
                <PasskeyDomainSelector
                  domains={props.passkeyDomains}
                  value={input.method === 'passkey' ? input.rpID : undefined}
                  onChange={(rpID) =>
                    props.onInputChange({ method: 'passkey', rpID })
                  }
                  disabled={verifying}
                />
                <p className='text-muted-foreground flex items-center gap-2 text-sm'>
                  <KeyRound className='size-5' />
                  {t(
                    'We will prompt your device to confirm using biometrics or your hardware key.'
                  )}
                </p>
              </TabsContent>
            </Tabs>
          )}
        </form>
      )}
    </Dialog>
  )
}

function EmailCodeFields(props: {
  inputId: string
  code: string
  scope?: string
  disabled: boolean
  onChange: (code: string) => void
}) {
  const { t } = useTranslation()
  const [sending, setSending] = useState(false)
  const [sent, setSent] = useState(false)
  const [sendError, setSendError] = useState<string>()
  const canRequest = Boolean(props.scope) && !props.disabled && !sending

  const requestCode = () => {
    if (!props.scope || !canRequest) return
    setSending(true)
    setSendError(undefined)
    void requestSecurityEmailCode(props.scope)
      .then(() => {
        setSent(true)
      })
      .catch((error) => {
        setSent(false)
        setSendError(AuthOperationError.from(error).message)
      })
      .finally(() => setSending(false))
  }

  return (
    <>
      <Label htmlFor={props.inputId}>{t('Email verification code')}</Label>
      <div className='flex gap-2'>
        <Input
          id={props.inputId}
          autoComplete='one-time-code'
          inputMode='numeric'
          maxLength={6}
          autoFocus
          disabled={props.disabled}
          value={props.code}
          onChange={(event) => props.onChange(event.target.value)}
          className='min-w-0 flex-1'
        />
        <Button
          type='button'
          variant='outline'
          disabled={!canRequest}
          onClick={requestCode}
        >
          {sending && <Loader2 className='size-4 animate-spin' />}
          {t('Send code')}
        </Button>
      </div>
      <p className='text-muted-foreground text-sm'>
        {t('Enter the code sent to your email. It can be used once.')}
      </p>
      {sent && !sendError && (
        <p role='status' className='text-sm'>
          {t('Code sent')}
        </p>
      )}
      {sendError && (
        <p role='alert' className='text-destructive text-sm'>
          {t(sendError)}
        </p>
      )}
    </>
  )
}
