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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import type { OwnerCustomer } from '@/features/adty-site-owner/types'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { handleServerError } from '@/lib/handle-server-error'

import { manageStaffCustomer, type StaffCustomerAction } from './api'

export function StaffCustomerActionDialog(props: {
  customer: OwnerCustomer
  action: StaffCustomerAction
  userId: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const verification = useSecureVerification()
  const active = useRef(true)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  const isReset = props.action === 'password-reset'
  const action = useMutation({
    retry: false,
    mutationFn: async () => {
      const proof = await verification.requestVerification({
        scope: 'site.staff.customer.manage',
        context: {
          action: isReset ? 'customer.password.reset' : 'customer.disable',
          target: props.customer.id,
        },
        title: t('Confirm it is you'),
        description: t(
          'Customer security changes require a fresh verification.'
        ),
      })
      if (!proof || !active.current) return false
      await manageStaffCustomer(
        props.customer.id,
        props.action,
        proof.proof_token
      )
      return true
    },
    onSuccess: (done) => {
      if (!done || !active.current) return
      toast.success(
        isReset
          ? t('Password reset instructions requested')
          : t('Customer disabled')
      )
      void client.invalidateQueries({ queryKey: ['adty-staff', props.userId] })
      props.onClose()
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <>
      <ConfirmDialog
        open
        onOpenChange={(open) => {
          if (!open && !action.isPending) props.onClose()
        }}
        title={
          isReset
            ? t('Send password reset instructions')
            : t('Disable customer')
        }
        desc={
          <div className='flex flex-col gap-2'>
            <span className='break-all'>{props.customer.username}</span>
            <span>
              {isReset
                ? t(
                    'Instructions are sent to the customer email. The customer chooses the new password; staff never receive a reset link.'
                  )
                : t(
                    'Existing customer sessions will stop working immediately. Restoring access requires an administrator.'
                  )}
            </span>
          </div>
        }
        destructive={!isReset}
        isLoading={action.isPending}
        handleConfirm={() => action.mutate()}
      />
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
