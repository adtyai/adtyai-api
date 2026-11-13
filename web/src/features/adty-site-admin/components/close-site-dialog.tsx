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
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { setSiteStatus, siteAdminKeys } from '../api'
import type { AdminSite } from '../types'

/**
 * Close a site (task 11.1, 11.9). Root confirms the close; customers are
 * inherited by the main site only. There is no option to divert them to
 * another site.
 */
export function CloseSiteDialog(props: {
  site: AdminSite
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const canClose = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const id = useId()
  const queryClient = useQueryClient()
  const [clawBack, setClawBack] = useState(false)
  const [immediate, setImmediate] = useState(false)
  const [reason, setReason] = useState('')

  const close = useMutation({
    mutationFn: () => {
      if (useAuthStore.getState().auth.user?.role !== ROLE.SUPER_ADMIN) {
        throw new Error(t('No permission to perform this action'))
      }
      return setSiteStatus(props.site.id, {
        status: 'closed',
        transfer_to: 0,
        claw_back: clawBack,
        immediate,
        reason: reason.trim(),
      })
    },
    onSuccess: () => {
      toast.success(t('Site closing scheduled'))
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
      props.onOpenChange(false)
    },
    onError: (err) => handleServerError(err),
  })

  if (!canClose) return null

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Close {{site}}', { site: props.site.host })}
      description={t(
        'Customers are told before the site closes. On the day it closes, they are inherited by the main site and the custom and sender domains are removed.'
      )}
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            variant='destructive'
            disabled={close.isPending}
            onClick={() => close.mutate()}
          >
            {t('Close site')}
          </Button>
        </>
      }
    >
      <div className='flex flex-col gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t('All customers of this site will be inherited by the main site.')}
        </p>
        <div className='flex items-center gap-2'>
          <Checkbox
            id={`${id}-claw`}
            checked={clawBack}
            onCheckedChange={(value) => setClawBack(value === true)}
          />
          <Label htmlFor={`${id}-claw`}>
            {t('Claw back unsettled and withdrawable earnings')}
          </Label>
        </div>
        <div className='flex items-center gap-2'>
          <Checkbox
            id={`${id}-now`}
            checked={immediate}
            onCheckedChange={(value) => setImmediate(value === true)}
          />
          <Label htmlFor={`${id}-now`}>
            {t('Close now without the notice period')}
          </Label>
        </div>
        <div className='space-y-1'>
          <Label htmlFor={`${id}-reason`}>{t('Reason')}</Label>
          <Textarea
            id={`${id}-reason`}
            rows={3}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        </div>
      </div>
    </Dialog>
  )
}
