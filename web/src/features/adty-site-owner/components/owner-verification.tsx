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
import React, { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  SecureVerificationDialog,
  useSecureVerification,
  type SiteOwnerAction,
} from '@/features/auth/secure-verification'

type OwnerVerifyContext = {
  /**
   * Asks the owner to verify for one sensitive action on one target and
   * returns the single-use proof token, or null when the owner cancels.
   */
  verify: (action: SiteOwnerAction, target: number) => Promise<string | null>
  verifying: boolean
}

const Context = React.createContext<OwnerVerifyContext | null>(null)

/** Mounts the shared step-up dialog once for the whole owner center. */
export function OwnerVerificationProvider(props: {
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const verification = useSecureVerification()
  const requestVerification = verification.requestVerification
  const verify = useCallback(
    async (action: SiteOwnerAction, target: number) => {
      const proof = await requestVerification({
        scope: 'site.owner.manage',
        context: { action, target },
        title: t('Confirm it is you'),
        description: t('Sensitive site changes require a fresh verification.'),
      })
      return proof?.proof_token ?? null
    },
    [requestVerification, t]
  )
  const value = useMemo(
    () => ({ verify, verifying: verification.isActive }),
    [verify, verification.isActive]
  )
  return (
    <Context value={value}>
      {props.children}
      <SecureVerificationDialog {...verification.dialogProps} />
    </Context>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useOwnerVerification(): OwnerVerifyContext {
  const value = React.useContext(Context)
  if (!value) {
    throw new Error(
      'useOwnerVerification must be used inside OwnerVerificationProvider'
    )
  }
  return value
}
