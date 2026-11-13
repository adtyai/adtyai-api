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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import {
  sideDrawerContentClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
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
import { DraftDiscardDialog } from '@/features/system-settings/components/draft-discard-dialog'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  adjustSite,
  clearSiteNotice,
  disableSiteDomain,
  disableSiteSenderDomain,
  getAdminBalanceRequests,
  getSite,
  getSiteCustomers,
  getSiteStats,
  reviewAdminBalanceRequest,
  getSites,
  resumeSiteSettlement,
  reviewSite,
  setSiteStatus,
  SITE_ADMIN_PAGE_SIZE,
  siteAdminKeys,
  type AdminBalanceRequest,
  type AdminSenderDomain,
  type AdminSiteDomain,
} from '../api'
import { VISIBILITY_KEYS } from '../lib/settings'
import {
  readSiteVisibility,
  serializeSiteVisibility,
  validSiteVisibility,
} from '../lib/site-visibility'
import { siteNotice } from '../lib/sites'
import type { AdminSite } from '../types'
import { CloseSiteDialog } from './close-site-dialog'

import { SiteStatusBadge } from './site-status-badge'
import { SiteVisibilityEditor } from './site-visibility-editor'

function Info(props: { label: string; value: string }) {
  return (
    <div className='flex flex-col gap-0.5'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='text-sm break-all'>{props.value || '-'}</dd>
    </div>
  )
}

/** Review, adjust and status actions of one site, by permission. */
function SiteActions(props: {
  site: AdminSite
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canClose = user?.role === ROLE.SUPER_ADMIN
  const canReview = user?.role === ROLE.SUPER_ADMIN
  const canAdjust = canAdmin(user, ADTY_PERMISSIONS.SITE_ADJUST)
  const canDelete = canAdmin(user, ADTY_PERMISSIONS.SITE_DELETE)
  const site = props.site
  const [costFactor, setCostFactor] = useState(site.cost_factor ?? '')
  const [maxMarkup, setMaxMarkup] = useState(site.max_markup ?? '')
  const [visibility, setVisibility] = useState(() =>
    readSiteVisibility(site.visibility)
  )
  const [reason, setReason] = useState('')

  const previousSite = useRef(site)
  useEffect(() => {
    const previous = previousSite.current
    setCostFactor((current) =>
      current === (previous.cost_factor ?? '')
        ? (site.cost_factor ?? '')
        : current
    )
    setMaxMarkup((current) =>
      current === (previous.max_markup ?? '')
        ? (site.max_markup ?? '')
        : current
    )
    const previousVisibility = readSiteVisibility(previous.visibility)
    const nextVisibility = readSiteVisibility(site.visibility)
    setVisibility((current) => {
      const merged = { ...current }
      for (const key of VISIBILITY_KEYS) {
        if (current[key] === previousVisibility[key]) {
          merged[key] = nextVisibility[key]
        }
      }
      return merged
    })
    previousSite.current = site
  }, [site])
  const savedVisibility = readSiteVisibility(site.visibility)
  const visibilityValid = validSiteVisibility(site.visibility)
  const visibilityChanged = VISIBILITY_KEYS.some(
    (key) => visibility[key] !== savedVisibility[key]
  )
  const adjustmentDirty =
    costFactor.trim() !== (site.cost_factor ?? '') ||
    maxMarkup.trim() !== (site.max_markup ?? '') ||
    visibilityChanged
  const dirty =
    costFactor !== (site.cost_factor ?? '') ||
    maxMarkup !== (site.max_markup ?? '') ||
    reason !== '' ||
    visibilityChanged
  const { onDirtyChange } = props
  useDraftNavigation(dirty)
  useEffect(() => {
    onDirtyChange(dirty)
    return () => onDirtyChange(false)
  }, [dirty, onDirtyChange])
  const [closing, setClosing] = useState(false)
  const [takingDown, setTakingDown] = useState(false)
  const targets = useQuery({
    queryKey: siteAdminKeys.list('transfer-targets', {}),
    queryFn: () => getSites({ p: 1, status: 'active', page_size: 100 }),
    enabled: closing && canClose,
  })
  const done = () => {
    toast.success(t('Saved successfully'))
    setReason('')
    void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
  }
  const review = useMutation({
    mutationFn: (approve: boolean) => {
      if (useAuthStore.getState().auth.user?.role !== ROLE.SUPER_ADMIN) {
        throw new Error(t('No permission to perform this action'))
      }
      return reviewSite(site.id, {
        approve,
        cost_factor: costFactor.trim() || undefined,
        max_markup: maxMarkup.trim() || undefined,
        note: reason.trim(),
      })
    },
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const adjust = useMutation({
    mutationFn: () => {
      if (
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.SITE_ADJUST
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      const body: Parameters<typeof adjustSite>[1] = { reason: reason.trim() }
      const clear: Array<'cost_factor' | 'max_markup'> = []
      if (costFactor.trim() !== (site.cost_factor ?? '')) {
        if (costFactor.trim() === '') clear.push('cost_factor')
        else body.cost_factor = costFactor.trim()
      }
      if (maxMarkup.trim() !== (site.max_markup ?? '')) {
        if (maxMarkup.trim() === '') clear.push('max_markup')
        else body.max_markup = maxMarkup.trim()
      }
      if (clear.length > 0) body.clear = clear
      if (visibilityChanged) {
        body.visibility = serializeSiteVisibility(visibility)
      }
      return adjustSite(site.id, body)
    },
    onSuccess: (updated) => {
      setCostFactor(updated.cost_factor ?? '')
      setMaxMarkup(updated.max_markup ?? '')
      setVisibility(readSiteVisibility(updated.visibility))
      queryClient.setQueryData<Awaited<ReturnType<typeof getSite>>>(
        siteAdminKeys.site(site.id),
        (cached) => (cached ? { ...cached, site: updated } : cached)
      )
      done()
    },
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
    mutationFn: () => {
      if (
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.SITE_DELETE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return clearSiteNotice(site.id, reason.trim())
    },
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

  const visibilityEditor = (
    <SiteVisibilityEditor
      value={visibility}
      onChange={setVisibility}
      disabled={
        !canAdjust ||
        !editable ||
        site.status === 'pending' ||
        busy ||
        !visibilityValid
      }
    />
  )
  if (!canReview && !canAdjust && !canDelete) return visibilityEditor
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
            disabled={(!canReview && !canAdjust) || !editable || busy}
            onChange={(event) => setCostFactor(event.target.value)}
          />
        </div>
        <div className='space-y-1'>
          <Label htmlFor='site-max-markup'>{t('Markup limit')}</Label>
          <Input
            id='site-max-markup'
            value={maxMarkup}
            placeholder={t('Global')}
            disabled={(!canReview && !canAdjust) || !editable || busy}
            onChange={(event) => setMaxMarkup(event.target.value)}
          />
        </div>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Leave empty to use the global value. The owner is notified when d changes.'
        )}
      </p>
      {visibilityEditor}
      {!visibilityValid && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Saved site visibility settings are invalid. Reload or repair the saved settings before editing this site.'
          )}
        </p>
      )}
      <div className='space-y-1'>
        <Label htmlFor='site-reason'>{t('Reason')}</Label>
        <Textarea
          id='site-reason'
          rows={2}
          disabled={busy}
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
            disabled={
              busy ||
              !adjustmentDirty ||
              reason.trim() === '' ||
              !visibilityValid
            }
            onClick={() => adjust.mutate()}
          >
            {t('Save site settings')}
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
        {canDelete && notice && (
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => setTakingDown(true)}
          >
            {t('Take down notice')}
          </Button>
        )}
        {canClose && editable && (
          <Button
            variant='destructive'
            disabled={busy}
            onClick={() => setClosing(true)}
          >
            {t('Close site')}
          </Button>
        )}
        {canReview && ['active', 'paused'].includes(site.status) && (
          <p className='text-muted-foreground text-sm'>
            {t('Site owners cannot be replaced')}
          </p>
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
        disabled={!canDelete}
        handleConfirm={() => canDelete && takedown.mutate()}
      />
      {canClose && closing && targets.isError && (
        <ErrorState onRetry={() => void targets.refetch()} />
      )}
      {canClose && closing && targets.isPending && <LoadingState />}
      {canClose && closing && targets.isSuccess && (
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
  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }
  return (
    <section className='flex flex-col gap-2'>
      <h3 className='text-sm font-medium'>{t('Customers')}</h3>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
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

function SiteDomains(props: {
  siteId: number
  domains: AdminSiteDomain[]
  senders: AdminSenderDomain[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canDelete = canAdmin(user, ADTY_PERMISSIONS.SITE_DELETE)
  const disableDomain = useMutation({
    mutationFn: (id: number) => {
      if (
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.SITE_DELETE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return disableSiteDomain(id)
    },
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const disableSender = useMutation({
    mutationFn: () => {
      if (
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.SITE_DELETE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return disableSiteSenderDomain(props.siteId)
    },
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  if (props.domains.length === 0 && props.senders.length === 0) return null
  return (
    <section className='flex flex-col gap-2'>
      <h3 className='text-sm font-medium'>{t('Custom domains')}</h3>
      {props.domains.map((domain) => (
        <div
          key={domain.id}
          className='flex items-center justify-between gap-2'
        >
          <span className='text-sm break-all'>
            {domain.hostname} · {domain.status}
          </span>
          {canDelete && (
            <Button
              variant='outline'
              size='sm'
              disabled={disableDomain.isPending}
              onClick={() => disableDomain.mutate(domain.id)}
            >
              {t('Disable')}
            </Button>
          )}
        </div>
      ))}
      {props.senders.map((sender) => (
        <div
          key={sender.domain}
          className='flex items-center justify-between gap-2'
        >
          <span className='text-sm break-all'>
            {sender.domain} · {sender.status}
          </span>
          {canDelete && (
            <Button
              variant='outline'
              size='sm'
              disabled={disableSender.isPending}
              onClick={() => disableSender.mutate()}
            >
              {t('Disable')} {t('Sender domain')}
            </Button>
          )}
        </div>
      ))}
    </section>
  )
}

function RootBalanceRequests(props: { siteId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const list = useQuery({
    queryKey: siteAdminKeys.list('balance-requests', { siteId: props.siteId }),
    queryFn: () => getAdminBalanceRequests(props.siteId),
    enabled: isRoot,
  })
  const review = useMutation({
    mutationFn: (input: { id: number; approve: boolean }) =>
      reviewAdminBalanceRequest(input.id, input.approve),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: siteAdminKeys.list('balance-requests', { siteId: props.siteId }),
      }),
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  return (
    <section className='flex flex-col gap-2'>
      <h3 className='text-sm font-medium'>{t('Balance requests')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'This request does not change the customer balance until the site owner or Root approves it.'
        )}
      </p>
      {list.isPending && <LoadingState />}
      {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
      {list.isSuccess && (
        <StaticDataTable
          tableClassName='min-w-[640px]'
          tableProps={{ 'aria-label': t('Balance requests') }}
          data={list.data}
          getRowKey={(row) => String(row.id)}
          emptyContent={<EmptyState className='min-h-24' title={t('No data')} />}
          columns={[
            {
              id: 'customer',
              header: t('Customer ID'),
              cell: (row: AdminBalanceRequest) => (
                <span className='tabular-nums'>{row.customer_user_id}</span>
              ),
            },
            {
              id: 'staff',
              header: t('User ID'),
              cell: (row: AdminBalanceRequest) => (
                <span className='tabular-nums'>{row.staff_user_id}</span>
              ),
            },
            {
              id: 'direction',
              header: t('Direction'),
              cell: (row: AdminBalanceRequest) =>
                row.direction === 'increase' ? t('Increase') : t('Decrease'),
            },
            {
              id: 'amount',
              header: t('Amount'),
              cell: (row: AdminBalanceRequest) => (
                <span className='tabular-nums'>{row.amount_usd}</span>
              ),
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (row: AdminBalanceRequest) =>
                row.status === 'pending'
                  ? t('Pending')
                  : row.status === 'approved'
                    ? t('Approved')
                    : t('Rejected'),
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (row: AdminBalanceRequest) =>
                row.status === 'pending' ? (
                  <div className='flex flex-wrap gap-1'>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={review.isPending}
                      onClick={() => review.mutate({ id: row.id, approve: true })}
                    >
                      {t('Approve')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={review.isPending}
                      onClick={() => review.mutate({ id: row.id, approve: false })}
                    >
                      {t('Reject')}
                    </Button>
                  </div>
                ) : (
                  <span className='text-muted-foreground text-xs tabular-nums'>
                    {row.reviewed_at > 0 ? formatTimestampToDate(row.reviewed_at) : '-'}
                  </span>
                ),
            },
          ]}
        />
      )}
    </section>
  )
}

/** Details of one site (task 11.9): info, actions, customers and the audit trail. */
export function SiteDetailSheet(props: {
  siteId: number | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [dirty, setDirty] = useState(false)
  const [discarding, setDiscarding] = useState(false)
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
    <Sheet
      open={props.siteId !== null}
      onOpenChange={(open) => {
        if (!open && dirty) setDiscarding(true)
        else props.onOpenChange(open)
      }}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{site?.host ?? t('Site')}</SheetTitle>
          <SheetDescription>
            {site ? <SiteStatusBadge status={site.status} /> : null}
          </SheetDescription>
        </SheetHeader>
        {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
        {!query.isError &&
          (!site ? (
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
                {statsQuery.isError && (
                  <ErrorState onRetry={() => void statsQuery.refetch()} />
                )}
                {statsQuery.isPending && <LoadingState />}
                {statsQuery.data && (
                  <dl className='grid gap-3 sm:grid-cols-4'>
                    <Info
                      label={t('Revenue')}
                      value={totals?.revenue_usd ?? '0'}
                    />
                    <Info
                      label={t('Earnings')}
                      value={totals?.earning_usd ?? '0'}
                    />
                    <Info
                      label={t('Top-up')}
                      value={totals?.topup_usd ?? '0'}
                    />
                    <Info
                      label={t('Withdrawn')}
                      value={statsQuery.data?.withdrawn_usd ?? '-'}
                    />
                  </dl>
                )}
              </section>
              <SiteActions key={site.id} site={site} onDirtyChange={setDirty} />
              <RootBalanceRequests siteId={site.id} />
              <SiteDomains
                siteId={site.id}
                domains={query.data?.domains ?? []}
                senders={query.data?.sender_domains ?? []}
              />
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
          ))}
      </SheetContent>
      <DraftDiscardDialog
        open={discarding}
        onOpenChange={setDiscarding}
        onDiscard={() => props.onOpenChange(false)}
      />
    </Sheet>
  )
}
