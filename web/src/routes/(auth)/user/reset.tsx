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
import { createFileRoute, useNavigate, useSearch } from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import {
  ResetPasswordConfirm,
  type ResetPasswordSearchParams,
} from '@/features/auth/reset-password-confirm'

export const Route = createFileRoute('/(auth)/user/reset')({
  validateSearch: (
    search: Record<string, unknown>
  ): ResetPasswordSearchParams => ({
    email: typeof search.email === 'string' ? search.email : undefined,
    token: typeof search.token === 'string' ? search.token : undefined,
  }),
  component: UserResetPassword,
})

function UserResetPassword() {
  const navigate = useNavigate()
  const search = useSearch({
    from: '/(auth)/user/reset',
  })
  const browser = window as Window & {
    __adtyReadPasswordResetLink?: () => ResetPasswordSearchParams
  }
  const [credentials, setCredentials] = useState(
    () => browser.__adtyReadPasswordResetLink?.() ?? search
  )
  useEffect(() => {
    const read = browser.__adtyReadPasswordResetLink
    delete browser.__adtyReadPasswordResetLink
    if (read || search.email !== undefined || search.token !== undefined) {
      setCredentials(read?.() ?? { email: search.email, token: search.token })
      void navigate({ to: '/user/reset', search: {}, hash: '', replace: true })
    }
  }, [browser, navigate, search.email, search.token])

  return (
    <ResetPasswordConfirm email={credentials.email} token={credentials.token} />
  )
}
