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
import { Copy, Info, RefreshCw, Share2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { useAffInvitees, useAffSelf } from './hooks/use-affiliate'
import { generateAffInviteLink } from './lib/invite-link'
import { formatAffRate, formatAffUsd } from './lib/quota'

export function AffiliateCenterPage() {
  const { t } = useTranslation()
  const self = useAffSelf()
  const [inviteOffset, setInviteOffset] = useState(0)
  const invitePage = 20
  const invitees = useAffInvitees(inviteOffset)
  const { copyToClipboard } = useCopyToClipboard()

  const data = self.data
  const inviteLink = useMemo(
    () => (data?.aff_code ? generateAffInviteLink(data.aff_code) : ''),
    [data?.aff_code]
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invite & rewards')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          className='h-8'
          render={<Link to='/affiliate/withdraw' />}
        >
          {t('Commissions & withdraw')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void self.refetch()}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Invite friends to register. You earn commission on their paid top-ups (gift amounts excluded).'
            )}
          </p>

          {self.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}

          {self.isError && !self.data ? (
            <ErrorState
              title={t('Failed to load affiliate summary')}
              onRetry={() => void self.refetch()}
            />
          ) : null}

          {data ? (
            <>
              <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
                {(
                  [
                    [t('Invites'), String(data.aff_count ?? 0), null],
                    [t('Total earned'), formatAffUsd(data.aff_history), null],
                    [
                      t('Available to withdraw'),
                      formatAffUsd(data.aff_quota),
                      `${t('Frozen')} ${formatAffUsd(data.frozen_quota)}`,
                    ],
                    [
                      t('Top-up commission rate'),
                      formatAffRate(data.topup_rate),
                      `${t('Freeze days')}: ${String(data.freeze_days ?? 0)}`,
                    ],
                    [
                      t('Subscription commission rate'),
                      formatAffRate(data.subscription_rate),
                      null,
                    ],
                  ] as [string, string, string | null][]
                ).map(([label, value, sub]) => (
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
                    {sub ? (
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {sub}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>

              <div className='grid gap-4 lg:grid-cols-[1fr_360px]'>
                <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                  <div className='flex items-center gap-2 border-b px-5 pt-4 pb-3'>
                    <Share2 className='text-primary size-4' />
                    <div className='text-[15px] font-semibold'>
                      {t('My invite')}
                    </div>
                  </div>
                  <div className='space-y-4 p-5'>
                    <div>
                      <div className='text-muted-foreground mb-1.5 text-[13px]'>
                        {t('Invite link')}
                      </div>
                      <div className='flex gap-2'>
                        <div className='bg-muted/40 flex h-10 flex-1 items-center overflow-hidden rounded-lg border px-3 font-mono text-[13px]'>
                          <span className='truncate'>{inviteLink || '-'}</span>
                        </div>
                        <Button
                          type='button'
                          className='h-10'
                          disabled={!inviteLink}
                          onClick={() => {
                            copyToClipboard(inviteLink)
                            toast.success(t('Copied'))
                          }}
                        >
                          <Copy className='size-4' />
                          {t('Copy link')}
                        </Button>
                      </div>
                    </div>
                    <div>
                      <div className='text-muted-foreground mb-1.5 text-[13px]'>
                        {t('Invite code')}
                      </div>
                      <div className='flex items-center gap-2'>
                        <span className='text-primary font-mono text-2xl font-semibold tracking-widest'>
                          {data.aff_code || '-'}
                        </span>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='h-7'
                          disabled={!data.aff_code}
                          onClick={() => {
                            copyToClipboard(data.aff_code)
                            toast.success(t('Copied'))
                          }}
                        >
                          <Copy className='size-3.5' />
                          {t('Copy')}
                        </Button>
                      </div>
                    </div>
                  </div>
                </section>

                <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                  <div className='border-b px-5 pt-4 pb-3'>
                    <div className='text-[15px] font-semibold'>
                      {t('Commission rules')}
                    </div>
                  </div>
                  <div className='space-y-3 p-5 text-[13px]'>
                    {(
                      [
                        [
                          t('Top-up commission rate'),
                          formatAffRate(data.topup_rate),
                        ],
                        [
                          t('Subscription commission rate'),
                          formatAffRate(data.subscription_rate),
                        ],
                        [
                          t('Freeze period'),
                          `${String(data.freeze_days ?? 0)} ${t('days')}`,
                        ],
                        [
                          t('Minimum withdraw'),
                          `$${Number(data.min_withdraw_usd || 0).toFixed(2)}`,
                        ],
                        [
                          t('Withdraw fee rate'),
                          formatAffRate(data.withdraw_fee_rate),
                        ],
                      ] as [string, string][]
                    ).map(([k, v]) => (
                      <div
                        key={k}
                        className='flex justify-between gap-3 border-b border-dashed pb-2'
                      >
                        <span className='text-muted-foreground'>{k}</span>
                        <span className='font-medium'>{v}</span>
                      </div>
                    ))}
                    <div className='bg-muted/40 text-muted-foreground flex gap-2.5 rounded-lg border px-3.5 py-2.5 text-[13px] leading-5'>
                      <Info className='mt-0.5 size-4 shrink-0' />
                      <div>
                        {t(
                          'Gift amounts are excluded. Commission base is the invitee paid amount.'
                        )}
                      </div>
                    </div>
                  </div>
                </section>
              </div>

              <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                <div className='border-b px-5 pt-4 pb-3'>
                  <div className='text-[15px] font-semibold'>
                    {t('Invitees')} ({invitees.data?.total ?? 0})
                  </div>
                  <div className='text-muted-foreground mt-0.5 text-xs'>
                    {t('Names are masked. Contact details are not shown.')}
                  </div>
                </div>
                <div className='divide-y'>
                  {(invitees.data?.rows ?? []).length === 0 ? (
                    <div className='text-muted-foreground px-5 py-6 text-sm'>
                      {invitees.isLoading
                        ? t('Loading...')
                        : t('No invitees')}
                    </div>
                  ) : (
                    invitees.data?.rows.map((row) => (
                      <div
                        key={row.user_id}
                        className='flex items-center justify-between px-5 py-3 text-sm'
                      >
                        <span>{row.name || '-'}</span>
                        <span className='text-muted-foreground text-xs'>
                          {row.registered_at
                            ? new Date(
                                row.registered_at > 1e12
                                  ? row.registered_at
                                  : row.registered_at * 1000
                              ).toLocaleDateString()
                            : '-'}
                        </span>
                      </div>
                    ))
                  )}
                </div>
                {(invitees.data?.total ?? 0) > invitePage ? (
                  <div className='flex justify-end gap-2 border-t px-5 py-3'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7'
                      disabled={inviteOffset <= 0}
                      onClick={() =>
                        setInviteOffset(Math.max(0, inviteOffset - invitePage))
                      }
                    >
                      {t('Previous')}
                    </Button>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7'
                      disabled={
                        inviteOffset + invitePage >=
                        (invitees.data?.total ?? 0)
                      }
                      onClick={() => setInviteOffset(inviteOffset + invitePage)}
                    >
                      {t('Next')}
                    </Button>
                  </div>
                ) : null}
              </section>
            </>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export default AffiliateCenterPage
