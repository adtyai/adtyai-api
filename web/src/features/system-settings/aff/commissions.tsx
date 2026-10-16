import { Info, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatAffRate, formatAffUsd } from '@/features/affiliate/lib/quota'
import { commissionStatusLabel } from '@/features/affiliate/lib/status'
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
import { commissionSourceLabelKey } from '@/features/system-settings/utils/status-labels'

import {
  ConfirmAffActionDialog,
  type AffConfirmKind,
} from './components/confirm-aff-action-dialog'
import { useAdminAffCommissions } from './hooks/use-admin-aff'
import type { AffCommission } from './types'

export function AffCommissionsSettings() {
  const { t } = useTranslation()
  const [status, setStatus] = useState('all')
  const list = useAdminAffCommissions(status === 'all' ? '' : status)
  const [target, setTarget] = useState<AffConfirmKind | null>(null)
  const isForbidden = (list.error as { status?: number } | null)?.status === 403
  const rows = list.data?.rows ?? []

  function canVoid(row: AffCommission) {
    return row.status === 'frozen' || row.status === 'available'
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Affiliate commissions')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Select
          items={[
            { value: 'all', label: t('All statuses') },
            { value: 'frozen', label: t('Frozen') },
            { value: 'available', label: t('Available') },
            { value: 'voided', label: t('Voided') },
            { value: 'blocked', label: t('Blocked') },
          ]}
          value={status}
          onValueChange={(v) => {
            if (v) setStatus(v)
          }}
        >
          <SelectTrigger className='h-8 w-40 text-sm'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('All statuses')}</SelectItem>
            <SelectItem value='frozen'>{t('Frozen')}</SelectItem>
            <SelectItem value='available'>{t('Available')}</SelectItem>
            <SelectItem value='voided'>{t('Voided')}</SelectItem>
            <SelectItem value='blocked'>{t('Blocked')}</SelectItem>
          </SelectContent>
        </Select>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void list.refetch()}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Admin commission ledger. Void deducts available quota. Requires a second confirmation.'
            )}
          </p>
          <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
            <Info className='mt-0.5 size-4 shrink-0' />
            <div>
              {t(
                'Blocked commissions (e.g. self-invite) cannot be voided here. Review self-invite cases on the withdrawals page.'
              )}
            </div>
          </div>

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'You do not have permission to view affiliate commissions.'
              )}
            />
          ) : null}

          {list.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}

          {list.isError && !isForbidden && !list.data ? (
            <ErrorState
              title={t('Failed to load commissions')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
              <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                <div className='text-[15px] font-semibold'>
                  {t('Commissions')}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {list.data.total} {t('records')}
                </div>
              </div>
              <Table>
                <TableHeader>
                  <TableRow className='bg-muted/40 hover:bg-muted/40'>
                    <TableHead>ID</TableHead>
                    <TableHead>{t('Inviter')}</TableHead>
                    <TableHead>{t('Invitee')}</TableHead>
                    <TableHead>{t('Order')}</TableHead>
                    <TableHead>{t('Paid base')}</TableHead>
                    <TableHead>{t('Rate')}</TableHead>
                    <TableHead>{t('Commission')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead className='w-[1%]' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className='text-muted-foreground h-24 text-center text-sm'
                      >
                        {t('No commissions')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => (
                      <TableRow key={row.id}>
                        <TableCell>
                          <code className='text-xs'>{row.id}</code>
                        </TableCell>
                        <TableCell>{row.inviter_id}</TableCell>
                        <TableCell>{row.invitee_id}</TableCell>
                        <TableCell>
                          <span className='font-mono text-[11.5px]'>
                            {t(commissionSourceLabelKey(row.source))}{' '}
                            {row.order_no}
                          </span>
                        </TableCell>
                        <TableCell>
                          {`$${Number(row.paid_amount_usd || 0).toFixed(2)}`}
                        </TableCell>
                        <TableCell>
                          {formatAffRate(row.rate_snapshot)}
                        </TableCell>
                        <TableCell className='font-semibold'>
                          {formatAffUsd(row.commission_quota)}
                        </TableCell>
                        <TableCell>
                          <code className='text-xs'>
                            {t(commissionStatusLabel(row.status))}
                          </code>
                        </TableCell>
                        <TableCell>
                          {canVoid(row) ? (
                            <div className='flex items-center gap-1.5'>
                              <Button
                                type='button'
                                variant='outline'
                                size='sm'
                                className='h-7 px-2.5 text-xs'
                                onClick={() => setTarget({ kind: 'void', row })}
                              >
                                {t('Void')}
                              </Button>
                            </div>
                          ) : (
                            <span className='text-muted-foreground text-xs'>
                              -
                            </span>
                          )}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </section>
          ) : null}
        </div>
      </SectionPageLayout.Content>

      <ConfirmAffActionDialog
        target={target}
        open={Boolean(target)}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
      />
    </SectionPageLayout>
  )
}

export default AffCommissionsSettings
