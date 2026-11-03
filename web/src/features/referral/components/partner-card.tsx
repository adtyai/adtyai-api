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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import { applyReferralPartner } from '../api'
import type { ReferralAffiliate } from '../types'

/** Identity: affiliate, partner (own rate) or disabled; affiliates may apply to be partners. */
export function PartnerCard(props: {
  affiliate: ReferralAffiliate
  onApplied: () => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [note, setNote] = useState('')
  const affiliate = props.affiliate
  const apply = useMutation({
    mutationFn: () => applyReferralPartner(note.trim()),
    onSuccess: () => {
      toast.success(t('Application submitted'))
      setOpen(false)
      props.onApplied()
    },
    onError: (err) => handleServerError(err),
  })

  let label = t('Affiliate')
  let variant: StatusVariant = 'info'
  if (affiliate.status === 'disabled') {
    label = t('Disabled')
    variant = 'neutral'
  } else if (affiliate.partner_status === 'approved') {
    label = t('Partner')
    variant = 'success'
  }
  const canApply =
    affiliate.status !== 'disabled' &&
    (affiliate.partner_status === '' || affiliate.partner_status === 'rejected')

  return (
    <Card>
      <CardContent className='flex flex-wrap items-center justify-between gap-3 py-4'>
        <div className='flex items-center gap-3'>
          <StatusBadge label={label} variant={variant} />
          <span className='text-muted-foreground text-sm'>
            {affiliate.partner_status === 'pending'
              ? t('Your partner application is under review.')
              : t('Partners with a large audience can get a higher rate.')}
          </span>
        </div>
        {canApply && (
          <Button size='sm' variant='outline' onClick={() => setOpen(true)}>
            {t('Apply to be a partner')}
          </Button>
        )}
      </CardContent>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Apply to be a partner')}
        description={t(
          'Tell us where you promote (site, channel, audience size).'
        )}
        footer={
          <Button
            disabled={note.trim() === '' || apply.isPending}
            onClick={() => apply.mutate()}
          >
            {t('Submit')}
          </Button>
        }
      >
        <div className='space-y-2'>
          <Label htmlFor='partner-note'>{t('About you')}</Label>
          <Textarea
            id='partner-note'
            value={note}
            maxLength={1000}
            onChange={(event) => setNote(event.target.value)}
          />
        </div>
      </Dialog>
    </Card>
  )
}
