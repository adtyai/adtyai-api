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
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { toIntlLocale } from '@/i18n/languages'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatNumber, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  cancelCampaign,
  deleteCampaign,
  getCampaign,
  getCampaigns,
  marketingKeys,
  type CampaignInput,
  type MarketingCampaign,
} from '../api'
import {
  CAMPAIGN_STATUS_LABELS,
  CAMPAIGN_STATUS_VARIANTS,
  campaignToInput,
  emptyCampaign,
} from '../lib/campaign'
import { CampaignEditor } from './campaign-editor'

type CampaignPanelProps = {
  templates: boolean
  canCreate: boolean
  canWrite: boolean
  canDelete: boolean
  canSend: boolean
  includeSitesDefault: boolean
}

type Editing = { id: number | null; input: CampaignInput }

/** Campaign or template list with stats, editing, cancel and "use template". */
export function CampaignPanel(props: CampaignPanelProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<Editing | null>(null)
  const [statsId, setStatsId] = useState<number | null>(null)
  const [canceling, setCanceling] = useState<MarketingCampaign | null>(null)
  const params = { p: page, is_template: props.templates }
  const query = useQuery({
    queryKey: marketingKeys.campaigns(params),
    queryFn: () => getCampaigns(params),
  })
  const stats = useQuery({
    queryKey: marketingKeys.campaign(statsId ?? 0),
    queryFn: () => getCampaign(statsId ?? 0),
    enabled: statsId !== null,
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: marketingKeys.all })
  const remove = useMutation({
    mutationFn: (id: number) => {
      if (
        !props.canDelete ||
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.MARKETING_DELETE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return deleteCampaign(id)
    },
    onSuccess: () => {
      toast.success(t('Deleted'))
      void refresh()
    },
    onError: (err) => handleServerError(err),
  })
  const cancel = useMutation({
    mutationFn: (id: number) => {
      if (
        !props.canSend ||
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.MARKETING_SEND
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return cancelCampaign(id)
    },
    onSuccess: () => {
      toast.success(t('Campaign canceled'))
      setCanceling(null)
      void refresh()
    },
    onError: (err) => handleServerError(err),
  })

  if (editing) {
    return (
      <CampaignEditor
        campaignId={editing.id}
        initial={editing.input}
        canCreate={props.canCreate}
        canWrite={props.canWrite}
        canSend={props.canSend}
        onClose={() => setEditing(null)}
      />
    )
  }

  const newInput = () => ({
    ...emptyCampaign(props.includeSitesDefault),
    is_template: props.templates,
  })

  return (
    <div className='flex flex-col gap-3'>
      {props.canCreate && (
        <div>
          <Button
            size='sm'
            onClick={() => setEditing({ id: null, input: newInput() })}
          >
            {props.templates ? t('New template') : t('New campaign')}
          </Button>
        </div>
      )}
      <StaticDataTable
        tableClassName='min-w-[860px]'
        tableProps={{
          'aria-label': props.templates ? t('Templates') : t('Campaigns'),
        }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState
            className='min-h-32'
            title={
              props.templates ? t('No templates yet') : t('No campaigns yet')
            }
          />
        }
        columns={[
          { id: 'name', header: t('Name'), cell: (row) => row.name },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) =>
              props.templates ? (
                t('Template')
              ) : (
                <StatusBadge
                  label={t(CAMPAIGN_STATUS_LABELS[row.status])}
                  variant={CAMPAIGN_STATUS_VARIANTS[row.status]}
                />
              ),
          },
          {
            id: 'recipients',
            header: t('Recipients'),
            cell: (row) => formatNumber(row.recipients, locale),
          },
          {
            id: 'time',
            header: t('Time'),
            cell: (row) =>
              formatTimestamp(
                row.sent_at || row.scheduled_at || row.created_at
              ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (row) => (
              <div className='flex flex-wrap gap-1'>
                {row.status === 'draft' ? (
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() =>
                      setEditing({ id: row.id, input: campaignToInput(row) })
                    }
                  >
                    {props.canWrite ? t('Edit') : t('View')}
                  </Button>
                ) : (
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => setStatsId(row.id)}
                  >
                    {t('Statistics')}
                  </Button>
                )}
                {props.templates && props.canCreate && (
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() =>
                      setEditing({
                        id: null,
                        input: {
                          ...campaignToInput(row),
                          is_template: false,
                          audience: {
                            include_site_customers: props.includeSitesDefault,
                          },
                        },
                      })
                    }
                  >
                    {t('Use template')}
                  </Button>
                )}
                {row.status === 'draft' && props.canDelete && (
                  <Button
                    size='sm'
                    variant='ghost'
                    disabled={remove.isPending}
                    onClick={() => remove.mutate(row.id)}
                  >
                    {t('Delete')}
                  </Button>
                )}
                {row.status === 'scheduled' && props.canSend && (
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => setCanceling(row)}
                  >
                    {t('Cancel')}
                  </Button>
                )}
              </div>
            ),
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={20}
        onPageChange={setPage}
      />
      <Dialog
        open={statsId !== null}
        onOpenChange={(open) => !open && setStatsId(null)}
        title={stats.data?.campaign.name ?? t('Statistics')}
      >
        {stats.data && (
          <dl className='grid grid-cols-2 gap-3 text-sm sm:grid-cols-4'>
            {(
              [
                ['Recipients', stats.data.stats.recipients],
                ['Delivered', stats.data.stats.delivered],
                ['Opens', stats.data.stats.opened],
                ['Clicks', stats.data.stats.clicked],
                ['Bounced', stats.data.stats.bounced],
                ['Complaints', stats.data.stats.complained],
                ['Unsubscribed', stats.data.stats.unsubscribed],
              ] as const
            ).map(([label, value]) => (
              <div key={label} className='rounded-md border p-2'>
                <dt className='text-muted-foreground text-xs'>{t(label)}</dt>
                <dd className='text-lg font-semibold'>
                  {formatNumber(value, locale)}
                </dd>
              </div>
            ))}
          </dl>
        )}
      </Dialog>
      <ConfirmDialog
        open={canceling !== null}
        onOpenChange={(open) => !open && setCanceling(null)}
        title={t('Cancel this campaign?')}
        desc={t(
          'Resend stops the scheduled emails that have not gone out yet.'
        )}
        destructive
        disabled={!props.canSend || canceling === null}
        isLoading={cancel.isPending}
        handleConfirm={() => {
          if (canceling && props.canSend) cancel.mutate(canceling.id)
        }}
      />
    </div>
  )
}
