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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatAffUsd } from '@/features/affiliate/lib/quota'
import {
  commissionStatusLabel,
  withdrawMethodLabel,
  withdrawStatusLabel,
} from '@/features/affiliate/lib/status'

import { adminGetAffUserDetail } from './api'

function personName(displayName?: string, username?: string, fallback?: number) {
  return displayName || username || (fallback ? String(fallback) : '-')
}

const detailPageSize = 20

function DetailPager({
  offset,
  total,
  onChange,
}: {
  offset: number
  total: number
  onChange: (next: number) => void
}) {
  const { t } = useTranslation()
  if (total <= detailPageSize) return null
  return (
    <div className='mt-1 flex justify-end gap-2'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='h-6 px-2 text-xs'
        disabled={offset <= 0}
        onClick={() => onChange(Math.max(0, offset - detailPageSize))}
      >
        {t('Previous')}
      </Button>
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='h-6 px-2 text-xs'
        disabled={offset + detailPageSize >= total}
        onClick={() => onChange(offset + detailPageSize)}
      >
        {t('Next')}
      </Button>
    </div>
  )
}

export function AffUserDetailButton({ userId }: { userId: number }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [inviteeOffset, setInviteeOffset] = useState(0)
  const [commissionOffset, setCommissionOffset] = useState(0)
  const [withdrawalOffset, setWithdrawalOffset] = useState(0)
  const [transferOffset, setTransferOffset] = useState(0)
  const detail = useQuery({
    queryKey: [
      'admin-aff-user',
      userId,
      inviteeOffset,
      commissionOffset,
      withdrawalOffset,
      transferOffset,
    ],
    queryFn: () =>
      adminGetAffUserDetail(userId, {
        invitee_offset: inviteeOffset,
        commission_offset: commissionOffset,
        withdrawal_offset: withdrawalOffset,
        transfer_offset: transferOffset,
      }),
    enabled: open && userId > 0,
    staleTime: 15_000,
  })
  const data = detail.data

  return (
    <>
      <Button
        type='button'
        variant='link'
        className='h-auto px-0 text-xs'
        onClick={() => setOpen(true)}
      >
        {t('View')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className='max-h-[80vh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              {t('View affiliate detail')} · {userId}
            </DialogTitle>
          </DialogHeader>
          {detail.isLoading ? (
            <div className='text-muted-foreground text-sm'>{t('Loading...')}</div>
          ) : null}
          {detail.isError ? (
            <div className='text-destructive text-sm'>
              {t('Failed to load affiliate summary')}
            </div>
          ) : null}
          {data ? (
            <div className='grid gap-4 text-sm'>
              <div className='grid grid-cols-2 gap-2 text-xs'>
                <div>
                  {personName(data.display_name, data.username, data.user_id)}
                </div>
                <div>
                  {t('Inviter')}: {data.inviter_name || data.inviter_id || '-'}
                </div>
                <div>
                  {t('Available to withdraw')}: {formatAffUsd(data.aff_quota)}
                </div>
                <div>
                  {t('Total earned')}: {formatAffUsd(data.aff_history)}
                </div>
              </div>
              <section>
                <div className='mb-1 font-medium'>
                  {t('Invitees')} ({data.invitee_total ?? data.invitees.length})
                </div>
                {data.invitees.length === 0 ? (
                  <div className='text-muted-foreground text-xs'>
                    {t('No invitees')}
                  </div>
                ) : (
                  data.invitees.map((row) => (
                    <div
                      key={row.user_id}
                      className='flex justify-between gap-3 border-b py-1 text-xs'
                    >
                      <span>{row.name || row.user_id}</span>
                      <span className='text-muted-foreground'>
                        {formatAffUsd(row.commission_quota)}
                      </span>
                    </div>
                  ))
                )}
                <DetailPager
                  offset={inviteeOffset}
                  total={data.invitee_total ?? data.invitees.length}
                  onChange={setInviteeOffset}
                />
              </section>
              <section>
                <div className='mb-1 font-medium'>
                  {t('Commissions')} ({data.commissions.length}/
                  {data.commission_total ?? data.commissions.length})
                </div>
                {data.commissions.length === 0 ? (
                  <div className='text-muted-foreground text-xs'>
                    {t('No commissions')}
                  </div>
                ) : (
                  data.commissions.map((row) => (
                    <div
                      key={row.id}
                      className='flex justify-between gap-3 border-b py-1 text-xs'
                    >
                      <span>
                        {row.invitee_name || row.invitee_id} ·{' '}
                        {t(commissionStatusLabel(row.status))}
                      </span>
                      <span>{formatAffUsd(row.commission_quota)}</span>
                    </div>
                  ))
                )}
                <DetailPager
                  offset={commissionOffset}
                  total={data.commission_total ?? data.commissions.length}
                  onChange={setCommissionOffset}
                />
              </section>
              <section>
                <div className='mb-1 font-medium'>
                  {t('Withdrawals')} ({data.withdrawals.length}/
                  {data.withdrawal_total ?? data.withdrawals.length})
                </div>
                {data.withdrawals.length === 0 ? (
                  <div className='text-muted-foreground text-xs'>
                    {t('No withdrawals')}
                  </div>
                ) : (
                  data.withdrawals.map((row) => (
                    <div
                      key={row.id}
                      className='flex justify-between gap-3 border-b py-1 text-xs'
                    >
                      <span>
                        {t(withdrawMethodLabel(row))} ·{' '}
                        {t(withdrawStatusLabel(row.status))}
                      </span>
                      <span>{formatAffUsd(row.amount_quota)}</span>
                    </div>
                  ))
                )}
                <DetailPager
                  offset={withdrawalOffset}
                  total={data.withdrawal_total ?? data.withdrawals.length}
                  onChange={setWithdrawalOffset}
                />
              </section>
              <section>
                <div className='mb-1 font-medium'>
                  {t('Transfer to balance')} ({data.transfers.length}/
                  {data.transfer_total ?? data.transfers.length})
                </div>
                {data.transfers.length === 0 ? (
                  <div className='text-muted-foreground text-xs'>-</div>
                ) : (
                  data.transfers.map((row) => (
                    <div
                      key={row.id}
                      className='flex justify-between gap-3 border-b py-1 text-xs'
                    >
                      <span>
                        {row.created_at
                          ? new Date(row.created_at).toLocaleString()
                          : '-'}
                      </span>
                      <span>{formatAffUsd(Math.abs(row.delta))}</span>
                    </div>
                  ))
                )}
                <DetailPager
                  offset={transferOffset}
                  total={data.transfer_total ?? data.transfers.length}
                  onChange={setTransferOffset}
                />
              </section>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}
