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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { sideDrawerContentClassName } from '@/components/drawer-layout'
import { EmptyState } from '@/components/empty-state'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getOwnerCustomer,
  getOwnerCustomerLogs,
  getOwnerCustomerTopUps,
  grantTrial,
  reportCustomer,
  savePricing,
  siteOwnerKeys,
} from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'
import { newIdempotencyKey } from '../lib/site-owner'

/** Customer drawer: profile, requests, top-ups, keys, trial credit, own price and report. */
export function CustomerSheet(props: {
  customerId: number | null
  onOpenChange: (open: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const id = props.customerId ?? 0
  const [trialAmount, setTrialAmount] = useState('')
  const [markup, setMarkup] = useState('')
  const [reason, setReason] = useState('')
  const detail = useQuery({
    queryKey: siteOwnerKeys.list('customer', { id }),
    queryFn: () => getOwnerCustomer(id),
    enabled: id > 0,
  })
  const logs = useQuery({
    queryKey: siteOwnerKeys.list('customer-logs', { id }),
    queryFn: () => getOwnerCustomerLogs(id, 1),
    enabled: id > 0,
    meta: { errorToast: false },
  })
  const topups = useQuery({
    queryKey: siteOwnerKeys.list('customer-topups', { id }),
    queryFn: () => getOwnerCustomerTopUps(id, 1),
    enabled: id > 0,
    meta: { errorToast: false },
  })
  const trial = useVerifiedOwnerMutation<{ amount: string; key: string }>({
    action: 'trial',
    target: () => id,
    run: (vars, proof) => grantTrial(id, vars.amount, vars.key, proof),
    successMessage: t('Trial credit sent'),
    onSuccess: () => setTrialAmount(''),
  })
  const price = useVerifiedOwnerMutation<string>({
    action: 'pricing',
    target: () => id,
    run: (value, proof) =>
      savePricing(
        {
          default_markup: value,
          expected_version: detail.data?.pricing_version ?? 0,
        },
        proof,
        id
      ),
    onSuccess: () => setMarkup(''),
  })
  const report = useMutation({
    mutationFn: () => reportCustomer(id, reason.trim()),
    onSuccess: () => {
      toast.success(t('Report sent to the platform'))
      setReason('')
    },
    onError: (err) => handleServerError(err),
  })
  const customer = detail.data?.customer
  const ownMarkup = detail.data?.prices?.find((row) => row.group === '')?.markup

  return (
    <Sheet open={props.customerId !== null} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-2xl')}>
        <SheetHeader>
          <SheetTitle>{customer?.username ?? t('Customer')}</SheetTitle>
          <SheetDescription>{customer?.email}</SheetDescription>
        </SheetHeader>
        {customer && (
          <div className='flex flex-col gap-4 px-4 pb-6'>
            <dl className='grid grid-cols-2 gap-2 text-sm'>
              <dt className='text-muted-foreground'>{t('Balance')}</dt>
              <dd>
                {customer.balance === undefined
                  ? t('Hidden by the platform')
                  : formatQuota(customer.balance)}
              </dd>
              <dt className='text-muted-foreground'>{t('Used')}</dt>
              <dd>
                {customer.used_quota === undefined
                  ? t('Hidden by the platform')
                  : formatQuota(customer.used_quota)}
              </dd>
              <dt className='text-muted-foreground'>{t('Signed up')}</dt>
              <dd>{formatTimestamp(customer.created_at)}</dd>
              <dt className='text-muted-foreground'>{t('Own price')}</dt>
              <dd>
                {ownMarkup ? `× ${Number(ownMarkup)}` : t('Site default')}
              </dd>
            </dl>
            <Tabs defaultValue='logs'>
              <TabsList>
                <TabsTrigger value='logs'>{t('Requests')}</TabsTrigger>
                <TabsTrigger value='topups'>{t('Top-ups')}</TabsTrigger>
                <TabsTrigger value='tokens'>{t('API keys')}</TabsTrigger>
              </TabsList>
              <TabsContent value='logs'>
                {logs.isError ? (
                  <EmptyState
                    className='min-h-24'
                    title={t('The platform does not show this information')}
                  />
                ) : (
                  <StaticDataTable
                    tableProps={{ 'aria-label': t('Requests') }}
                    data={logs.data?.items ?? []}
                    getRowKey={(row) => String(row.id)}
                    emptyContent={
                      <EmptyState
                        className='min-h-24'
                        title={t('No requests yet')}
                      />
                    }
                    columns={[
                      {
                        id: 'time',
                        header: t('Time'),
                        cell: (row) => formatTimestamp(row.created_at),
                      },
                      {
                        id: 'model',
                        header: t('Model'),
                        cell: (row) => row.model_name,
                      },
                      {
                        id: 'tokens',
                        header: t('Tokens'),
                        cell: (row) =>
                          `${row.prompt_tokens} / ${row.completion_tokens}`,
                      },
                      {
                        id: 'quota',
                        header: t('Charged'),
                        cell: (row) => formatQuota(row.quota),
                      },
                    ]}
                  />
                )}
              </TabsContent>
              <TabsContent value='topups'>
                {topups.isError ? (
                  <EmptyState
                    className='min-h-24'
                    title={t('The platform does not show this information')}
                  />
                ) : (
                  <StaticDataTable
                    tableProps={{ 'aria-label': t('Top-ups') }}
                    data={topups.data?.items ?? []}
                    getRowKey={(row) => row.trade_no}
                    emptyContent={
                      <EmptyState
                        className='min-h-24'
                        title={t('No top-ups yet')}
                      />
                    }
                    columns={[
                      {
                        id: 'time',
                        header: t('Time'),
                        cell: (row) => formatTimestamp(row.created_at),
                      },
                      {
                        id: 'face',
                        header: t('Amount'),
                        cell: (row) => formatUsd(Number(row.face_usd), locale),
                      },
                      {
                        id: 'bonus',
                        header: t('Bonus'),
                        cell: (row) => formatUsd(Number(row.bonus_usd), locale),
                      },
                      {
                        id: 'status',
                        header: t('Status'),
                        cell: (row) => row.status,
                      },
                    ]}
                  />
                )}
              </TabsContent>
              <TabsContent value='tokens'>
                {detail.data?.tokens === undefined ? (
                  <EmptyState
                    className='min-h-24'
                    title={t('The platform does not show this information')}
                  />
                ) : (
                  <StaticDataTable
                    tableProps={{ 'aria-label': t('API keys') }}
                    data={detail.data.tokens}
                    getRowKey={(row) => String(row.id)}
                    emptyContent={
                      <EmptyState
                        className='min-h-24'
                        title={t('No API keys yet')}
                      />
                    }
                    columns={[
                      {
                        id: 'name',
                        header: t('Name'),
                        cell: (row) => row.name,
                      },
                      {
                        id: 'used',
                        header: t('Last used'),
                        cell: (row) =>
                          row.accessed_time > 0
                            ? formatTimestamp(row.accessed_time)
                            : '—',
                      },
                    ]}
                  />
                )}
              </TabsContent>
            </Tabs>
            <section className='space-y-2' aria-labelledby='trial-title'>
              <h3 id='trial-title' className='text-sm font-medium'>
                {t('Send trial credit')}
              </h3>
              <div className='flex items-end gap-2'>
                <div className='space-y-2'>
                  <Label htmlFor='trial-amount'>{t('Amount (USD)')}</Label>
                  <Input
                    id='trial-amount'
                    type='number'
                    min={0}
                    step='0.01'
                    value={trialAmount}
                    onChange={(event) => setTrialAmount(event.target.value)}
                  />
                </div>
                <Button
                  disabled={!trialAmount || trial.isPending}
                  onClick={() =>
                    trial.mutate({
                      amount: trialAmount,
                      key: newIdempotencyKey(),
                    })
                  }
                >
                  {t('Send')}
                </Button>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('Trial credit is taken from your earnings.')}
              </p>
            </section>
            <section className='space-y-2' aria-labelledby='own-price-title'>
              <h3 id='own-price-title' className='text-sm font-medium'>
                {t('Price for this customer')}
              </h3>
              <div className='flex items-end gap-2'>
                <div className='space-y-2'>
                  <Label htmlFor='own-markup'>{t('Markup')}</Label>
                  <Input
                    id='own-markup'
                    type='number'
                    min={1}
                    step='0.01'
                    value={markup}
                    onChange={(event) => setMarkup(event.target.value)}
                  />
                </div>
                <Button
                  variant='outline'
                  disabled={!markup || price.isPending}
                  onClick={() => price.mutate(markup)}
                >
                  {t('Save')}
                </Button>
              </div>
            </section>
            <section className='space-y-2' aria-labelledby='report-title'>
              <h3 id='report-title' className='text-sm font-medium'>
                {t('Report this customer')}
              </h3>
              <Textarea
                aria-labelledby='report-title'
                maxLength={1000}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
              <Button
                variant='outline'
                disabled={!reason.trim() || report.isPending}
                onClick={() => report.mutate()}
              >
                {t('Send report')}
              </Button>
            </section>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
