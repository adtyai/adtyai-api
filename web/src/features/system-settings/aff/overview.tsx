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
import { Link } from '@tanstack/react-router'
import { RefreshCw } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatAffUsd } from '@/features/affiliate/lib/quota'

import { adminGetAffOverview, adminListAffInviters } from './api'
import { AffUserDetailButton } from './user-detail-dialog'

export function AffOverviewSettings() {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [submitted, setSubmitted] = useState('')
  const [offset, setOffset] = useState(0)
  const limit = 20
  const overview = useQuery({
    queryKey: ['admin-aff-overview'],
    queryFn: adminGetAffOverview,
    staleTime: 15_000,
  })
  const inviters = useQuery({
    queryKey: ['admin-aff-inviters', submitted, offset],
    queryFn: () =>
      adminListAffInviters({ keyword: submitted, limit, offset }),
    staleTime: 15_000,
  })
  const data = overview.data
  const isForbidden =
    (overview.error as { status?: number } | null)?.status === 403

  const cards: [string, string][] = data
    ? [
        [t('Invited users'), String(data.invited_users ?? 0)],
        [t('Inviters'), String(data.inviters ?? 0)],
        [t('Frozen commission'), formatAffUsd(data.commission_frozen)],
        [t('Available commission'), formatAffUsd(data.commission_available)],
        [t('Voided commission'), formatAffUsd(data.commission_voided)],
        [t('Blocked commission'), formatAffUsd(data.commission_blocked)],
        [t('Pending USDT'), String(data.withdraw_pending_usdt ?? 0)],
        [t('Pending Alipay'), String(data.withdraw_pending_alipay ?? 0)],
        [t('Pending legacy'), String(data.withdraw_pending_legacy ?? 0)],
        [t('Paid withdrawal amount'), formatAffUsd(data.withdraw_paid_quota)],
      ]
    : []

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Affiliate overview')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          className='h-8'
          render={<Link to='/system-settings/aff-commissions' />}
        >
          {t('Affiliate commissions')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          className='h-8'
          render={<Link to='/system-settings/aff-withdrawals' />}
        >
          {t('Affiliate withdrawals')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => {
            void overview.refetch()
            void inviters.refetch()
          }}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Balance transfers are not withdrawal orders. Open a user detail to see them.'
            )}
          </p>
          {overview.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}
          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'You do not have permission to view affiliate withdrawals.'
              )}
            />
          ) : null}
          {overview.isError && !isForbidden && !data ? (
            <ErrorState
              title={t('Failed to load affiliate summary')}
              onRetry={() => void overview.refetch()}
            />
          ) : null}
          {data ? (
            <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
              {cards.map(([label, value]) => (
                <div
                  key={label}
                  className='bg-card rounded-xl border p-4 shadow-xs'
                >
                  <div className='text-muted-foreground text-[13px]'>
                    {label}
                  </div>
                  <div className='mt-2 text-[22px] font-semibold tracking-tight'>
                    {value}
                  </div>
                </div>
              ))}
            </div>
          ) : null}
          <div className='space-y-3'>
            <div className='flex gap-2'>
              <Input
                value={keyword}
                placeholder={t('Search inviters')}
                onChange={(event) => setKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key !== 'Enter') return
                  setOffset(0)
                  setSubmitted(keyword.trim())
                }}
              />
              <Button
                type='button'
                variant='outline'
                onClick={() => {
                  setOffset(0)
                  setSubmitted(keyword.trim())
                }}
              >
                {t('Search')}
              </Button>
            </div>
            {inviters.isError ? (
              <ErrorState
                title={t('Failed to load inviters')}
                onRetry={() => void inviters.refetch()}
              />
            ) : (
              <div className='overflow-x-auto rounded-md border'>
                <table className='w-full text-sm'>
                  <thead className='bg-muted/40 text-left'>
                    <tr>
                      <th className='px-3 py-2 font-medium'>{t('Inviter')}</th>
                      <th className='px-3 py-2 font-medium'>{t('Invite code')}</th>
                      <th className='px-3 py-2 font-medium'>{t('Invitees')}</th>
                      <th className='px-3 py-2 font-medium'>{t('Available to withdraw')}</th>
                      <th className='px-3 py-2 font-medium'>{t('Total earned')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(inviters.data?.rows ?? []).length === 0 ? (
                      <tr>
                        <td className='text-muted-foreground px-3 py-4' colSpan={5}>
                          {inviters.isLoading ? t('Loading...') : t('No inviters')}
                        </td>
                      </tr>
                    ) : (
                      inviters.data?.rows.map((row) => (
                        <tr key={row.user_id} className='border-t'>
                          <td className='px-3 py-2'>
                            <AffUserDetailButton userId={row.user_id}>
                              {row.display_name || row.username || row.user_id}
                            </AffUserDetailButton>
                          </td>
                          <td className='px-3 py-2 font-mono'>{row.aff_code || '-'}</td>
                          <td className='px-3 py-2'>{row.invitee_count}</td>
                          <td className='px-3 py-2'>{formatAffUsd(row.aff_quota)}</td>
                          <td className='px-3 py-2'>{formatAffUsd(row.aff_history)}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            )}
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={offset === 0}
                onClick={() => setOffset((current) => Math.max(0, current - limit))}
              >
                {t('Previous')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={offset + limit >= (inviters.data?.total ?? 0)}
                onClick={() => setOffset((current) => current + limit)}
              >
                {t('Next')}
              </Button>
            </div>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export default AffOverviewSettings
