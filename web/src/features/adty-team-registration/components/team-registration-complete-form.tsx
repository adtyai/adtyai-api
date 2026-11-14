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

import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'

import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usernameSchema } from '@/features/auth/constants'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  completeTeamRegistration,
  TEAM_REGISTRATION_CODE_MIN_LENGTH,
} from '../api'

function createdTeamName(data: unknown): string {
  if (!data || typeof data !== 'object') return ''
  const record = data as {
    Team?: { name?: unknown }
    team?: { name?: unknown }
  }
  const name = record.Team?.name ?? record.team?.name
  return typeof name === 'string' ? name : ''
}

export function TeamRegistrationCompleteForm(props: { code: string }) {
  const { t } = useTranslation()
  const code = props.code.trim()
  const invalidLink = code.length < TEAM_REGISTRATION_CODE_MIN_LENGTH
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [createdName, setCreatedName] = useState<string | null>(null)

  const usernameError = useMemo(() => {
    const result = usernameSchema('Please enter your username').safeParse(
      username
    )
    return result.success ? '' : result.error.issues[0]?.message
  }, [username])

  if (invalidLink) {
    return (
      <p className='text-destructive text-sm'>
        {t('This team registration link is invalid')}
      </p>
    )
  }

  if (createdName !== null) {
    return (
      <div className='flex flex-col gap-4'>
        <p className='text-sm'>
          {t('Your team has been created. You can sign in with this username.')}
        </p>
        {createdName !== '' && (
          <p className='text-sm'>
            {t('Team name')}: {createdName}
          </p>
        )}
        <Link
          to='/sign-in'
          className='hover:text-primary font-medium underline underline-offset-4'
        >
          {t('Sign in')}
        </Link>
      </div>
    )
  }

  const submit = async () => {
    const parsedUsername = usernameSchema('Please enter your username').safeParse(
      username
    )
    if (!parsedUsername.success) {
      setError(t(parsedUsername.error.issues[0]?.message || 'Invalid parameters'))
      return
    }
    if (name.trim() === '') {
      setError(t('Please enter the team name'))
      return
    }
    if (password === '') {
      setError(t('Please enter your password'))
      return
    }
    if (confirmPassword === '') {
      setError(t('Please confirm your password'))
      return
    }
    if (password !== confirmPassword) {
      setError(t("Passwords don't match."))
      return
    }
    setPending(true)
    setError('')
    try {
      const created = await completeTeamRegistration({
        code,
        username: parsedUsername.data,
        password,
        name: name.trim(),
      })
      setCreatedName(createdTeamName(created))
    } catch (cause) {
      setError(t(getServerErrorMessage(cause, t('Invalid parameters'))))
    } finally {
      setPending(false)
    }
  }

  return (
    <form
      className='flex flex-col gap-4'
      onSubmit={(event) => {
        event.preventDefault()
        void submit()
      }}
    >
      <div className='flex flex-col gap-1'>
        <Label htmlFor='team-registration-username'>{t('Username')}</Label>
        <Input
          id='team-registration-username'
          autoComplete='username'
          value={username}
          maxLength={20}
          onChange={(event) => setUsername(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='team-registration-password'>{t('Password')}</Label>
        <PasswordInput
          id='team-registration-password'
          autoComplete='new-password'
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='team-registration-confirm-password'>
          {t('Confirm password')}
        </Label>
        <PasswordInput
          id='team-registration-confirm-password'
          autoComplete='new-password'
          value={confirmPassword}
          onChange={(event) => setConfirmPassword(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='team-registration-team-name'>{t('Team name')}</Label>
        <Input
          id='team-registration-team-name'
          value={name}
          maxLength={64}
          onChange={(event) => setName(event.target.value)}
        />
      </div>
      {error && <p className='text-destructive text-sm'>{error}</p>}
      {username !== '' && usernameError && (
        <p className='text-muted-foreground text-sm'>{t(usernameError)}</p>
      )}
      <Button type='submit' disabled={pending}>
        {t('Register team')}
      </Button>
    </form>
  )
}
