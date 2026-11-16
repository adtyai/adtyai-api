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
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { SecureVerificationDialog } from '@/features/auth/secure-verification/components/secure-verification-dialog'
import { useSecureVerification } from '@/features/auth/secure-verification/hooks/use-secure-verification'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getSiteEarningReviews,
  resolveSiteEarningReview,
  type SiteEarningDecision,
  type SiteEarningReview,
} from '../api'

const PAGE_SIZE = 20

/**
 * Historical task earnings whose original charge record is missing. Finance
 * decides once per task whether the original charge counts toward the site's
 * earnings; until then the site's positive earnings stay on hold.
 */
export function SiteEarningReviewPanel(props: { canResolve: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [reviewing, setReviewing] = useState<SiteEarningReview | null>(null)
  const query = useQuery({
    queryKey: ['recharge-audit', 'site-earning-reviews', page],
    queryFn: () => getSiteEarningReviews(page, PAGE_SIZE),
  })
  const rows = Array.isArray(query.data?.items) ? query.data.items : []
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('Site earnings needing review')}</CardTitle>
          <CardDescription>
            {t(
              'Historical task adjustments whose original charge was not recorded. Until finance decides whether the original charge counts toward site earnings, that site keeps its positive earnings on hold.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
          {query.isPending ? (
            <LoadingState />
          ) : (
            query.data && (
              <>
                <StaticDataTable
                  tableClassName='min-w-[760px]'
                  tableProps={{
                    'aria-label': t('Site earnings needing review'),
                  }}
                  data={rows}
                  getRowKey={(row) => row.operation_key}
                  emptyContent={
                    <EmptyState
                      className='min-h-24'
                      title={t('Nothing to review')}
                    />
                  }
                  columns={[
                    {
                      id: 'time',
                      header: t('Time'),
                      cell: (row) => formatTimestamp(row.created_at),
                    },
                    {
                      id: 'site',
                      header: t('Site ID'),
                      cell: (row) => row.site_id,
                    },
                    {
                      id: 'customer',
                      header: t('Customer ID'),
                      cell: (row) => row.user_id,
                    },
                    {
                      id: 'task',
                      header: t('Task'),
                      cell: (row) => (
                        <code className='font-mono text-xs break-all'>
                          {row.root_operation_key}
                        </code>
                      ),
                    },
                    {
                      id: 'amount',
                      header: t('Site earnings'),
                      cellClassName: 'tabular-nums',
                      cell: (row) => formatQuota(row.quota),
                    },
                    {
                      id: 'actions',
                      header: t('Actions'),
                      className: 'text-right',
                      cellClassName: 'text-right',
                      cell: (row) =>
                        props.canResolve ? (
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => setReviewing(row)}
                          >
                            {t('Review')}
                          </Button>
                        ) : null,
                    },
                  ]}
                />
                <PageSwitcher
                  page={page}
                  total={Number(query.data.total) || 0}
                  pageSize={PAGE_SIZE}
                  onPageChange={setPage}
                />
              </>
            )
          )}
        </CardContent>
      </Card>
      {reviewing && (
        <SiteEarningReviewDialog
          key={reviewing.operation_key}
          event={reviewing}
          onClose={() => setReviewing(null)}
          onDone={() =>
            void queryClient.invalidateQueries({ queryKey: ['recharge-audit'] })
          }
        />
      )}
    </>
  )
}

function SiteEarningReviewDialog(props: {
  event: SiteEarningReview
  onClose: () => void
  onDone: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const verification = useSecureVerification()
  const [decision, setDecision] = useState<SiteEarningDecision | ''>('')
  const [reason, setReason] = useState('')
  const mutation = useMutation({
    mutationFn: async () => {
      if (!decision) return false
      const body = {
        operation_key: props.event.operation_key,
        original_charge: decision,
        reason: reason.trim(),
      }
      const proof = await verification.requestVerification({
        scope: 'adty.site_earning.review',
        context: body,
        title: t('Confirm it is you'),
        description: t('Deciding site earnings requires a fresh verification.'),
      })
      if (!proof?.proof_token) return false
      await resolveSiteEarningReview(body, proof.proof_token)
      return true
    },
    onSuccess: (done) => {
      if (!done) return
      toast.success(t('Review saved'))
      props.onDone()
      props.onClose()
    },
    onError: (error) => handleServerError(error),
  })
  const ready =
    decision !== '' && reason.trim() !== '' && reason.trim().length <= 1024
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
      title={t('Review site earnings')}
      description={t(
        'The decision applies to every adjustment of this task and cannot be changed afterwards.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            disabled={mutation.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            variant='brand'
            disabled={!ready || mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {t('Save review')}
          </Button>
        </>
      }
    >
      <SecureVerificationDialog {...verification.dialogProps} />
      <dl className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm'>
        <dt className='text-muted-foreground'>{t('Site ID')}</dt>
        <dd className='text-right tabular-nums'>{props.event.site_id}</dd>
        <dt className='text-muted-foreground'>{t('Customer ID')}</dt>
        <dd className='text-right tabular-nums'>{props.event.user_id}</dd>
        <dt className='text-muted-foreground'>{t('Task')}</dt>
        <dd className='text-right font-mono text-xs break-all'>
          {props.event.root_operation_key}
        </dd>
        <dt className='text-muted-foreground'>{t('Site earnings')}</dt>
        <dd className='text-right tabular-nums'>
          {formatQuota(props.event.quota)}
        </dd>
      </dl>
      <div className='space-y-2'>
        <Label htmlFor={`${id}-decision`}>{t('Original charge')}</Label>
        <NativeSelect
          id={`${id}-decision`}
          className='w-full'
          value={decision}
          onChange={(event) =>
            setDecision(event.target.value as SiteEarningDecision | '')
          }
        >
          <NativeSelectOption value=''>{t('Choose')}</NativeSelectOption>
          <NativeSelectOption value='released'>
            {t('Counts toward site earnings')}
          </NativeSelectOption>
          <NativeSelectOption value='skipped'>
            {t('Does not count toward site earnings')}
          </NativeSelectOption>
        </NativeSelect>
      </div>
      <div className='space-y-2'>
        <Label htmlFor={`${id}-reason`}>{t('Reason (required)')}</Label>
        <Textarea
          id={`${id}-reason`}
          value={reason}
          rows={3}
          onChange={(event) => setReason(event.target.value)}
        />
      </div>
    </Dialog>
  )
}
