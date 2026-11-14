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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getPausedSiteRepairs, repairPausedSite, siteAdminKeys } from '../api'
import type { AdminSite } from '../types'
import { SiteStatusBadge } from './site-status-badge'

/** Root-only review of leftover paused rows. Nothing is flipped automatically. */
export function PausedSiteRepairPanel() {
  const { t } = useTranslation()
  const isRoot =
    useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const queryClient = useQueryClient()
  const [repairing, setRepairing] = useState<AdminSite | null>(null)
  const [closing, setClosing] = useState<AdminSite | null>(null)
  const [reason, setReason] = useState('')
  const [immediate, setImmediate] = useState(false)
  const query = useQuery({
    queryKey: siteAdminKeys.list('paused-repairs', {}),
    queryFn: getPausedSiteRepairs,
    enabled: isRoot,
  })
  const repair = useMutation({
    mutationFn: (input: {
      site: AdminSite
      outcome: 'active' | 'close'
      reason: string
      immediate?: boolean
    }) => {
      if (useAuthStore.getState().auth.user?.role !== ROLE.SUPER_ADMIN) {
        throw new Error(t('No permission to perform this action'))
      }
      return repairPausedSite(input.site.id, {
        outcome: input.outcome,
        reason: input.reason.trim(),
        immediate: input.immediate,
      })
    },
    onSuccess: (_site, input) => {
      toast.success(
        input.outcome === 'active'
          ? t('Site repaired to active')
          : t('Site closing scheduled')
      )
      setRepairing(null)
      setClosing(null)
      setReason('')
      setImmediate(false)
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })

  if (!isRoot) return null
  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }
  const rows = query.data ?? []
  if (query.isLoading && rows.length === 0) return null
  if (rows.length === 0) return null

  return (
    <section
      className='bg-muted/40 flex flex-col gap-3 rounded-md border p-3'
      aria-label={t('Leftover paused sites')}
    >
      <div className='space-y-1'>
        <h2 className='text-sm font-medium'>{t('Leftover paused sites')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'These leftover paused sites are not an operating mode. Review each one and either repair it to active or enter close/handover. Nothing is changed automatically.'
          )}
        </p>
      </div>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <ul className='flex flex-col gap-2'>
        {rows.map((site) => (
          <li
            key={site.id}
            className='flex flex-wrap items-center justify-between gap-2'
          >
            <div className='flex flex-wrap items-center gap-2'>
              <span className='font-medium'>{site.host || site.subdomain}</span>
              <SiteStatusBadge status={site.status} />
            </div>
            <div className='flex flex-wrap gap-2'>
              <Button
                size='sm'
                variant='outline'
                disabled={repair.isPending}
                onClick={() => {
                  setReason('')
                  setRepairing(site)
                }}
              >
                {t('Repair to active')}
              </Button>
              <Button
                size='sm'
                variant='destructive'
                disabled={repair.isPending}
                onClick={() => {
                  setReason('')
                  setImmediate(false)
                  setClosing(site)
                }}
              >
                {t('Enter close/handover')}
              </Button>
            </div>
          </li>
        ))}
      </ul>
      <ConfirmDialog
        open={repairing !== null}
        onOpenChange={(open) => {
          if (!open) setRepairing(null)
        }}
        title={t('Repair to active')}
        desc={t(
          'Repair this leftover paused site to active? Pause will not remain an operating mode.'
        )}
        confirmText={t('Repair to active')}
        isLoading={repair.isPending}
        handleConfirm={() => {
          if (!repairing) return
          repair.mutate({
            site: repairing,
            outcome: 'active',
            reason,
          })
        }}
      >
        <Textarea
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          placeholder={t('Reason')}
          aria-label={t('Reason')}
        />
      </ConfirmDialog>
      <Dialog
        open={closing !== null}
        onOpenChange={(open) => {
          if (!open) setClosing(null)
        }}
        title={t('Enter close/handover')}
        description={t(
          'All customers of this site will be inherited by the main site.'
        )}
        footer={
          <>
            <Button variant='outline' onClick={() => setClosing(null)}>
              {t('Cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={repair.isPending}
              onClick={() => {
                if (!closing) return
                repair.mutate({
                  site: closing,
                  outcome: 'close',
                  reason,
                  immediate,
                })
              }}
            >
              {t('Enter close/handover')}
            </Button>
          </>
        }
      >
        <div className='flex flex-col gap-3'>
          <Textarea
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t('Reason')}
            aria-label={t('Reason')}
          />
          <div className='flex items-center gap-2'>
            <Checkbox
              id='paused-repair-immediate'
              checked={immediate}
              onCheckedChange={(checked) => setImmediate(checked === true)}
            />
            <Label htmlFor='paused-repair-immediate'>
              {t('Close now without the notice period')}
            </Label>
          </div>
        </div>
      </Dialog>
    </section>
  )
}
