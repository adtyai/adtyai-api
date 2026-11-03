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
import {
  sideDrawerContentClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  adjustSite,
  clearSiteNotice,
  getSite,
  getSiteCustomers,
  getSiteStats,
  getSites,
  resumeSiteSettlement,
  reviewSite,
  setSiteStatus,
  SITE_ADMIN_PAGE_SIZE,
  siteAdminKeys,
} from '../api'
import { siteNotice } from '../lib/sites'
import type { AdminSite } from '../types'
import { CloseSiteDialog } from './close-site-dialog'
import { SiteStatusBadge } from './site-status-badge'

function Info(props: { label: string; value: string }) {
  return (
    <div className='flex flex-col gap-0.5'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='text-sm break-all'>{props.value || '-'}</dd>
    </div>
  )
}

/** Review, adjust and status actions of one site, by permission. */
function SiteActions(props: { site: AdminSite }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canReview = canAdmin(user, ADTY_PERMISSIONS.SITE_REVIEW)
  const canAdjust = canAdmin(user, ADTY_PERMISSIONS.SITE_ADJUST)
  const site = props.site
  const [costFactor, setCostFactor] = useState(site.cost_factor ?? '')
  const [maxMarkup, setMaxMarkup] = useState(site.max_markup ?? '')
  const [reason, setReason] = useState('')
  const [closing, setClosing] = useState(false)
  const [takingDown, setTakingDown] = useState(false)
  const targets = useQuery({
    queryKey: siteAdminKeys.list('transfer-targets', {}),
    queryFn: () => getSites({ p: 1, status: 'active', page_size: 100 }),
    enabled: closing,
  })
  const done = () => {
    toast.success(t('Saved successfully'))
    setReason('')
    void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
  }
  const review = useMutation({
    mutationFn: (approve: boolean) =>
      reviewSite(site.id, {
        approve,
        cost_factor: costFactor.trim() || undefined,
        max_markup: maxMarkup.trim() || undefined,
        note: reason.trim(),
      }),
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const adjust = useMutation({
    mutationFn: () => {
      const clear: Array<'cost_factor' | 'max_markup'> = []
      if (costFactor.trim() === '' && site.cost_factor !== null) {
        clear.push('cost_factor')
      }
      if (maxMarkup.trim() === '' && site.max_markup !== null) {
        clear.push('max_markup')
      }
      return adjustSite(site.id, {
        cost_factor: costFactor.trim() || undefined,
        max_markup: maxMarkup.trim() || undefined,
        clear: clear.length > 0 ? clear : undefined,
        reason: reason.trim(),
      })
    },
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const status = useMutation({
    mutationFn: (next: 'active' | 'paused') =>
      setSiteStatus(site.id, { status: next, reason: reason.trim() }),
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const settlement = useMutation({
    mutationFn: () => resumeSiteSettlement(site.id),
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const takedown = useMutation({

    mutationFn: () => clearSiteNotice(site.id, reason.trim()),
    onSuccess: () => {
      setTakingDown(false)
      done()
    },
    onError: (err) => handleServerError(err),
  })
  const notice = siteNotice(site.brand)
  const busy =
    review.isPending ||
    adjust.isPending ||
    status.isPending ||
    settlement.isPending ||
    takedown.isPending
  const editable = site.status !== 'closed' && site.status !== 'closing'

  if (!canReview && !canAdjust) return null
  return (
    <section className='flex flex-col gap-3 rounded-lg border p-3'>
      <h3 className='text-sm font-medium'>{t('Manage')}</h3>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-1'>
          <Label htmlFor='site-cost-factor'>{t('Cost factor')}</Label>
          <Input
            id='site-cost-factor'
            value={costFactor}
            placeholder={t('Global')}
            disabled={!editable || busy}
            onChange={(event) => setCostFactor(event.target.value)}
          />
        </div>
        <div className='space-y-1'>
          <Label htmlFor='site-max-markup'>{t('Markup limit')}</Label>
          <Input
            id='site-max-markup'
            value={maxMarkup}
            placeholder={t('Global')}
            disabled={!editable || busy}
            onChange={(event) => setMaxMarkup(event.target.value)}
          />
        </div>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Leave empty to use the global value. The owner is notified when d changes.'
        )}
      </p>
      <div className='space-y-1'>
        <Label htmlFor='site-reason'>{t('Reason')}</Label>
        <Textarea
          id='site-reason'
          rows={2}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
      </div>
      <div className='flex flex-wrap gap-2'>
        {canReview && site.status === 'pending' && (
          <>
            <Button disabled={busy} onClick={() => review.mutate(true)}>
              {t('Approve')}
            </Button>
            <Button
              variant='outline'
              disabled={busy}
              onClick={() => review.mutate(false)}
            >
              {t('Reject')}
            </Button>
          </>
        )}
        {canAdjust && editable && site.status !== 'pending' && (
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => adjust.mutate()}
          >
            {t('Save d and k')}
          </Button>
        )}
        {canAdjust && site.status === 'active' && (
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => status.mutate('paused')}
          >
            {t('Pause')}
          </Button>
        )}
        {canAdjust && site.status === 'paused' && (
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => status.mutate('active')}
          >
            {t('Resume')}
          </Button>
        )}
        {canAdjust && (site.settle_hold_reason ?? '') !== '' && (
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => settlement.mutate()}
          >
            {t('Resume settlement')}
          </Button>
        )}
        {(site.settle_hold_reason ?? '') !== '' && (
          <p className='text-warning w-full text-sm'>
            {t('Settlement is on hold: {{reason}}', {
              reason: site.settle_hold_reason,
            })}
          </p>
        )}
        {canAdjust && notice && (
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => setTakingDown(true)}
          >
            {t('Take down notice')}
          </Button>
        )}
        {canAdjust && editable && (
          <Button
            variant='destructive'
            disabled={busy}
            onClick={() => setClosing(true)}
          >
            {t('Close site')}
          </Button>
        )}
      </div>
      {notice && (
        <div className='space-y-1'>
          <span className='text-muted-foreground text-xs'>
            {t('Current notice')}
          </span>
          <p className='bg-muted rounded p-2 text-sm whitespace-pre-wrap'>
            {notice}
          </p>
        </div>
      )}
      <ConfirmDialog
        open={takingDown}
        onOpenChange={setTakingDown}
        title={t('Take down notice')}
        desc={t(
          "The notice disappears from the site at once. The owner's other brand settings stay."
        )}
        destructive
        isLoading={takedown.isPending}
        handleConfirm={() => takedown.mutate()}
      />
      {closing && (
        <CloseSiteDialog
          site={site}
          transferTargets={targets.data?.items ?? []}
          open={closing}
          onOpenChange={setClosing}
        />
      )}
    </section>
  )
}

function SiteCustomers(props: { siteId: number }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: siteAdminKeys.list('customers', { id: props.siteId, page }),
    queryFn: () => getSiteCustomers(props.siteId, page),
  })
  const riskLabel = (flag: string) => {
    if (flag === 'self_use') return t('Possible self-use')
    if (flag === 'migration') return t('Possible main site migration')
    return '-'
  }
  return (
    <section className='flex flex-col gap-2'>
      <h3 className='text-sm font-medium'>{t('Customers')}</h3>
      <StaticDataTable
        tableClassName='min-w-[640px]'
        tableProps={{ 'aria-label': t('Customers') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={<EmptyState className='min-h-24' title={t('No data')} />}
        columns={[
          { id: 'user', header: t('User'), cell: (row) => row.username },
          { id: 'email', header: t('Email'), cell: (row) => row.email || '-' },
          {
            id: 'quota',
            header: t('Balance'),
            cell: (row) => formatQuotaWithCurrency(row.quota),
          },
          {
            id: 'used',
            header: t('Used'),
            cell: (row) => formatQuotaWithCurrency(row.used_quota),
          },
          {
            id: 'risk',
            header: t('Risk'),
            cell: (row) => riskLabel(row.risk_flag),
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={SITE_ADMIN_PAGE_SIZE}
        onPageChange={setPage}
      />
    </section>
  )
}

/** Details of one site (task 11.9): info, actions, customers and the audit trail. */
export function SiteDetailSheet(props: {
  siteId: number | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: siteAdminKeys.site(props.siteId ?? 0),
    queryFn: () => getSite(props.siteId ?? 0),
    enabled: props.siteId !== null,
  })
  const site = query.data?.site
  const statsQuery = useQuery({
    queryKey: siteAdminKeys.list('stats', { id: props.siteId ?? 0 }),
    queryFn: () => getSiteStats(props.siteId ?? 0),
    enabled: props.siteId !== null,
  })
  const totals = statsQuery.data?.stats.total

  return (
    <Sheet open={props.siteId !== null} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{site?.host ?? t('Site')}</SheetTitle>
          <SheetDescription>
            {site ? <SiteStatusBadge status={site.status} /> : null}
          </SheetDescription>
        </SheetHeader>
        {!site ? (
          <LoadingState />
        ) : (
          <div className='flex flex-col gap-5 overflow-y-auto p-4'>
            <dl className='grid gap-3 sm:grid-cols-3'>
              <Info
                label={t('Owner')}
                value={site.owner_username || `#${site.owner_user_id}`}
              />
              <Info
                label={t('Opened')}
                value={formatTimestampToDate(site.created_at)}
              />
              <Info label={t('Customers')} value={String(site.customers)} />
              <Info
                label={t('Cost factor')}
                value={site.cost_factor ?? t('Global')}
              />
              <Info
                label={t('Tier cost factor')}
                value={
                  site.tier_cost_factor
                    ? `${site.tier_cost_factor} (${site.tier_month})`
                    : '-'
                }
              />
              <Info
                label={t('Markup limit')}
                value={site.max_markup ?? t('Global')}
              />
              <Info label={t('Owner contact')} value={site.contact_info} />
              {site.close_execute_at > 0 && (
                <Info
                  label={t('Closes on')}
                  value={formatTimestampToDate(site.close_execute_at)}
                />
              )}
            </dl>
            <section className='flex flex-col gap-2'>
              <h3 className='text-sm font-medium'>{t('Business data')}</h3>
              <dl className='grid gap-3 sm:grid-cols-4'>
                <Info label={t('Revenue')} value={totals?.revenue_usd ?? '0'} />
                <Info label={t('Earnings')} value={totals?.earning_usd ?? '0'} />
                <Info label={t('Top-up')} value={totals?.topup_usd ?? '0'} />
                <Info
                  label={t('Withdrawn')}
                  value={statsQuery.data?.withdrawn_usd ?? '-'}
                />
              </dl>
            </section>
            <SiteActions site={site} />
            <SiteCustomers siteId={site.id} />
            <section className='flex flex-col gap-2'>
              <h3 className='text-sm font-medium'>{t('Audit trail')}</h3>
              <StaticDataTable
                tableClassName='min-w-[600px]'
                tableProps={{ 'aria-label': t('Audit trail') }}
                data={query.data?.audits ?? []}
                getRowKey={(row) => String(row.id)}
                emptyContent={
                  <EmptyState className='min-h-24' title={t('No data')} />
                }
                columns={[
                  {
                    id: 'time',
                    header: t('Time'),
                    cell: (row) => formatTimestampToDate(row.created_at),
                  },
                  {
                    id: 'action',
                    header: t('Action'),
                    cell: (row) => (
                      <span className='font-mono text-xs'>{row.action}</span>
                    ),
                  },
                  {
                    id: 'operator',
                    header: t('Operator'),
                    cell: (row) => `#${row.operator_id}`,
                  },
                  {
                    id: 'change',
                    header: t('Change'),
                    cell: (row) => (
                      <span className='font-mono text-xs break-all'>
                        {row.old_value || '-'} → {row.new_value || '-'}
                      </span>
                    ),
                  },
                  {
                    id: 'reason',
                    header: t('Reason'),
                    cell: (row) => row.reason || '-',
                  },
                ]}
              />
            </section>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
