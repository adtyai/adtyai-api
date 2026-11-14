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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { invalidateSidebarBadges } from '@/hooks/use-sidebar-badges'

import type { SiteOwnerAction } from '@/features/auth/secure-verification'
import { handleServerError } from '@/lib/handle-server-error'

import { siteOwnerKeys } from '../api'
import { useOwnerVerification } from '../components/owner-verification'

/**
 * A sensitive owner write: verify first (single-use proof bound to the action
 * and target), then call the API with the proof. Cancelling the verification
 * is not an error. On success the owner center queries are refreshed.
 */
export function useVerifiedOwnerMutation<TVars>(options: {
  action: SiteOwnerAction
  target: (vars: TVars) => number
  run: (vars: TVars, proof: string) => Promise<unknown>
  successMessage?: string
  onSuccess?: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const owner = useOwnerVerification()
  return useMutation({
    mutationFn: async (vars: TVars) => {
      const proof = await owner.verify(options.action, options.target(vars))
      if (!proof) return false
      await options.run(vars, proof)
      return true
    },
    onSuccess: (done) => {
      if (!done) return
      toast.success(options.successMessage ?? t('Saved successfully'))
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all })
      void invalidateSidebarBadges(queryClient)
      options.onSuccess?.()
    },
    onError: (err) => handleServerError(err),
  })
}
