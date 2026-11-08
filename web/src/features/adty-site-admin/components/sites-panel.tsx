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
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { toIntlLocale } from '@/i18n/languages'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  createSite,
  getSites,
  SITE_ADMIN_PAGE_SIZE,
  siteAdminKeys,
} from '../api'
import {
  DEFAULT_SITE_SORT,
  nextSiteSort,
  type SiteSort,
  type SiteSortKey,
} from '../lib/sites'
import type { AdminSite, SiteStatus } from '../types'
import { SiteStatusBadge } from './site-status-badge'

const STATUSES: SiteStatus[] = [
  'pending',
  'active',
  'paused',
  'closing',
  'closed',
]

/** Column header that sorts the loaded page; aria-sort sits on the header cell. */
function SortHeader(props: {
  label: string
  sortKey: SiteSortKey
  sort: SiteSort
  onSort: (key: SiteSortKey) => void
}) {
  const active = props.sort.key === props.sortKey
  let icon: ReactNode = <ArrowUpDown className='size-3.5' aria-hidden />
  if (active) {
    icon =
      props.sort.direction === 'asc' ? (
        <ArrowUp className='size-3.5' aria-hidden />
      ) : (
        <ArrowDown className='size-3.5' aria-hidden />
      )
  }
  return (
    <Button
      variant='ghost'
      size='sm'
      className='-ml-2 h-7 px-2'
      onClick={() => props.onSort(props.sortKey)}
    >
      {props.label}
      {icon}
    </Button>
  )
}

function ariaSort(sort: SiteSort, key: SiteSortKey) {
  if (sort.key !== key) return undefined
  return sort.direction === 'asc' ? 'ascending' : 'descending'
}

/** Site list (task 11.9): status filter, search, sortable columns, paging. */
export function SitesPanel(props: { onOpenSite: (site: AdminSite) => void }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('')
  const [keyword, setKeyword] = useState('')
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SiteSort>(DEFAULT_SITE_SORT)
  const [ownerId, setOwnerId] = useState('')
  const [subdomain, setSubdomain] = useState('')
  const canCreate = canAdmin(
    useAuthStore((state) => state.auth.user),
    ADTY_PERMISSIONS.SITE_CREATE
  )
  const queryClient = useQueryClient()
  const params = {
    p: page,
    status,
    keyword: search,
    sort: sort.key,
    order: sort.direction,
  }
  const query = useQuery({
    queryKey: siteAdminKeys.list('sites', params),
    queryFn: () => getSites(params),
  })
  const rows = query.data?.items ?? []
  const openSite = useMutation({
    mutationFn: () => {
      if (
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.SITE_CREATE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return createSite({
        user_id: Number(ownerId),
        subdomain: subdomain.trim(),
      })
    },
    onSuccess: () => {
      setOwnerId('')
      setSubdomain('')
      toast.success(t('Site opened'))
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const onSort = (key: SiteSortKey) => {
    setSort((prev) => nextSiteSort(prev, key))
    setPage(1)
  }
  const statusLabels: Record<SiteStatus, string> = {
    pending: t('Pending review'),
    active: t('Active'),
    paused: t('Paused'),
    closing: t('Closing'),
    closed: t('Closed'),
  }

  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }

  return (
    <div className='flex flex-col gap-3'>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <form
        className='flex flex-wrap gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          setSearch(keyword.trim())
          setPage(1)
        }}
      >
        <NativeSelect
          aria-label={t('Status')}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value)
            setPage(1)
          }}
        >
          <NativeSelectOption value=''>{t('All statuses')}</NativeSelectOption>
          {STATUSES.map((item) => (
            <NativeSelectOption key={item} value={item}>
              {statusLabels[item]}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <Input
          className='w-64'
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder={t('Subdomain or owner username')}
          aria-label={t('Search')}
        />
        <Button type='submit'>{t('Search')}</Button>
      </form>
      {canCreate && (
        <form
          className='flex flex-wrap items-end gap-2'
          aria-label={t('Open a reseller site')}
          onSubmit={(event) => {
            event.preventDefault()
            if (!canCreate) return
            openSite.mutate()
          }}
        >
          <Input
            className='w-32'
            inputMode='numeric'
            value={ownerId}
            onChange={(event) => setOwnerId(event.target.value)}
            aria-label={t('User ID')}
            placeholder={t('User ID')}
            required
          />
          <Input
            className='w-48'
            value={subdomain}
            onChange={(event) => setSubdomain(event.target.value)}
            aria-label={t('Domain')}
            placeholder={t('Domain')}
            required
          />
          <Button type='submit' disabled={openSite.isPending}>
            {t('Open a reseller site')}
          </Button>
        </form>
      )}
      {/* The ui/table primitives directly: StaticDataTable cannot put
          aria-sort on the header cell of a sortable column. */}
      <div className='overflow-x-auto rounded-lg border'>
        <Table className='min-w-[860px]' aria-label={t('Reseller sites')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Site')}</TableHead>
              <TableHead>{t('Owner')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Cost factor')}</TableHead>
              <TableHead>{t('Markup limit')}</TableHead>
              <TableHead aria-sort={ariaSort(sort, 'customers')}>
                <SortHeader
                  label={t('Customers')}
                  sortKey='customers'
                  sort={sort}
                  onSort={onSort}
                />
              </TableHead>
              <TableHead aria-sort={ariaSort(sort, 'topup')}>
                <SortHeader
                  label={t('Top-up')}
                  sortKey='topup'
                  sort={sort}
                  onSort={onSort}
                />
              </TableHead>
              <TableHead aria-sort={ariaSort(sort, 'revenue')}>
                <SortHeader
                  label={t('Revenue')}
                  sortKey='revenue'
                  sort={sort}
                  onSort={onSort}
                />
              </TableHead>
              <TableHead aria-sort={ariaSort(sort, 'earning')}>
                <SortHeader
                  label={t('Earnings')}
                  sortKey='earning'
                  sort={sort}
                  onSort={onSort}
                />
              </TableHead>
              <TableHead aria-sort={ariaSort(sort, 'growth')}>
                <SortHeader
                  label={t('Growth')}
                  sortKey='growth'
                  sort={sort}
                  onSort={onSort}
                />
              </TableHead>
              <TableHead>{t('Close requested')}</TableHead>
              <TableHead>{t('Opened')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={13}>
                  <EmptyState className='min-h-32' title={t('No data')} />
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow key={row.id}>
                  <TableCell className='font-mono'>{row.host}</TableCell>
                  <TableCell>
                    {row.owner_username || `#${row.owner_user_id}`}
                  </TableCell>
                  <TableCell>
                    <SiteStatusBadge status={row.status} />
                  </TableCell>
                  <TableCell>
                    {row.cost_factor ?? row.tier_cost_factor ?? t('Global')}
                  </TableCell>
                  <TableCell>{row.max_markup ?? t('Global')}</TableCell>
                  <TableCell>{formatNumber(row.customers, locale)}</TableCell>
                  <TableCell>{row.month_topup_usd ?? '0'}</TableCell>
                  <TableCell>{row.month_revenue_usd ?? '0'}</TableCell>
                  <TableCell>{row.month_earning_usd ?? '0'}</TableCell>
                  <TableCell>
                    {row.growth_pct == null ? '-' : `${row.growth_pct}%`}
                  </TableCell>
                  <TableCell>
                    {row.close_requested_at > 0
                      ? formatTimestampToDate(row.close_requested_at)
                      : '-'}
                  </TableCell>
                  <TableCell>{formatTimestampToDate(row.created_at)}</TableCell>
                  <TableCell className='text-right'>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => props.onOpenSite(row)}
                    >
                      {t('Details')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={SITE_ADMIN_PAGE_SIZE}
        onPageChange={setPage}
      />
    </div>
  )
}
