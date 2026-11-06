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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getFlaggedCustomers,
  getSiteReports,
  handleSiteReport,
  reviewFlaggedCustomer,
  siteAdminKeys,
} from '../api'

/** Flagged customers and owner reports. Resolving a report calls the review API. */
export function ReviewsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const flagged = useQuery({
    queryKey: siteAdminKeys.list('flagged', { p: 1 }),
    queryFn: () => getFlaggedCustomers(1),
  })
  const reports = useQuery({
    queryKey: siteAdminKeys.list('reports', { p: 1 }),
    queryFn: () => getSiteReports(1),
  })
  const review = useMutation({
    mutationFn: (userId: number) => reviewFlaggedCustomer(userId, true),
    onSuccess: () => {
      toast.success(t('Review this customer'))
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const resolve = useMutation({
    mutationFn: (id: number) =>
      handleSiteReport(id, { status: 'resolved', note: '' }),
    onSuccess: () => {
      toast.success(t('Resolve report'))
      void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const flaggedRows = flagged.data?.items ?? []
  const reportRows = reports.data?.items ?? []

  return (
    <div className='flex flex-col gap-6'>
      <section className='flex flex-col gap-2'>
        <h3 className='text-sm font-medium'>{t('Risk')}</h3>
        {flaggedRows.length === 0 ? (
          <EmptyState className='min-h-24' title={t('No data')} />
        ) : (
          <ul>
            {flaggedRows.map((row) => (
              <li
                key={`${row.site_id}-${row.user_id}`}
                className='flex items-center justify-between gap-2'
              >
                <span>{row.risk_reason || row.risk_flag}</span>
                <Button
                  size='sm'
                  onClick={() => review.mutate(row.user_id)}
                  disabled={row.reviewed || review.isPending}
                >
                  {t('Review this customer')}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section className='flex flex-col gap-2'>
        <h3 className='text-sm font-medium'>{t('Reports')}</h3>
        {reportRows.length === 0 ? (
          <EmptyState className='min-h-24' title={t('No data')} />
        ) : (
          <ul className='flex flex-col gap-2'>
            {reportRows.map((row) => (
              <li key={row.id} className='flex items-center justify-between gap-2'>
                <span>{row.reason}</span>
                <Button
                  size='sm'
                  onClick={() => resolve.mutate(row.id)}
                  disabled={resolve.isPending}
                >
                  {t('Resolve report')}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
