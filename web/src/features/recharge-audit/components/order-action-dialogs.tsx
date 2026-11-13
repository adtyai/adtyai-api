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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  officialCommissionStateLabel,
} from '@/features/wallet/adty/commission-states'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { SecureVerificationDialog } from '@/features/auth/secure-verification/components/secure-verification-dialog'
import { useSecureVerification } from '@/features/auth/secure-verification/hooks/use-secure-verification'

import {
  completeTopupManually,
  listTopupManualRefunds,
  previewTopupRefund,
  queryTopupPayment,
  refundTopup,
  runTopupManualRefundStep,
  type RechargeAuditOrder,
  type TopupRefundResult,
} from '../api'

type OrderActionProps = {
  order: RechargeAuditOrder | null
  onOpenChange: (open: boolean) => void
  onDone: () => void
}

export function QueryPaymentDialog(props: OrderActionProps) {
  const { t } = useTranslation()
  const mutation = useMutation({
    meta: { errorToast: false },
    mutationFn: () => {
      if (
        !props.order?.actions.query ||
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.FINANCE_MANUAL_CREDIT
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return queryTopupPayment(props.order.trade_no)
    },
    onSuccess: () => props.onDone(),
  })
  const resetQuery = mutation.reset
  useEffect(() => resetQuery(), [props.order?.trade_no, resetQuery])
  const completed = mutation.isSuccess && mutation.data.status === 'success'
  return (
    <ConfirmDialog
      open={props.order !== null}
      onOpenChange={props.onOpenChange}
      title={t('Query payment')}
      desc={t(
        'Query the original payment provider. A confirmed payment is credited using the saved order details.'
      )}
      confirmText={t('Query payment')}
      handleConfirm={() => mutation.mutate()}
      isLoading={mutation.isPending}
      disabled={completed}
    >
      <code className='text-xs break-all'>{props.order?.trade_no}</code>
      {mutation.isError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {getServerErrorMessage(mutation.error)}
          </AlertDescription>
        </Alert>
      )}
      {mutation.isSuccess && (
        <Alert>
          <AlertDescription>
            {completed ? t('Order completed') : t('No confirmed payment found')}
          </AlertDescription>
        </Alert>
      )}
    </ConfirmDialog>
  )
}

/** Manual completion after checking the payment at the provider. */
export function ManualCompleteDialog(props: OrderActionProps) {
  const { t } = useTranslation()
  const [proof, setProof] = useState('')
  const [note, setNote] = useState('')
  useEffect(() => {
    setProof('')
    setNote('')
  }, [props.order])
  const mutation = useMutation({
    mutationFn: () =>
      completeTopupManually(
        props.order?.trade_no ?? '',
        proof.trim(),
        note.trim()
      ),
    onSuccess: () => {
      toast.success(t('Order completed'))
      props.onDone()
      props.onOpenChange(false)
    },
    onError: (error) => handleServerError(error),
  })
  const ready = proof.trim() !== '' && note.trim() !== ''
  return (
    <Dialog
      open={props.order !== null}
      onOpenChange={props.onOpenChange}
      title={t('Complete order manually')}
      description={t(
        'Check the payment in the provider console first. The order is credited with its face value and bonus.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!ready || mutation.isPending}
          >
            {t('Complete')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label htmlFor='manual-proof'>
          {t('Proof (transaction hash or platform order number)')}
        </Label>
        <Input
          id='manual-proof'
          value={proof}
          onChange={(e) => setProof(e.target.value)}
        />
      </div>
      <div className='space-y-2'>
        <Label htmlFor='manual-note'>{t('Note')}</Label>
        <Textarea
          id='manual-note'
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={3}
        />
      </div>
    </Dialog>
  )
}

/**
 * Refund: operators apply, approve, record internal reclaim or shortfall,
 * record an offline payout, and complete. External payment is offline only.
 */
export function RefundDialog(props: OrderActionProps) {
  const { t } = useTranslation()
  const verification = useSecureVerification()
  const [reason, setReason] = useState('')
  const [refundUsd, setRefundUsd] = useState('')
  const [idempotencyKey, setIdempotencyKey] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [payoutCurrency, setPayoutCurrency] = useState('')
  const [payoutAmount, setPayoutAmount] = useState('')
  const [payoutAt, setPayoutAt] = useState('')
  const [payoutVoucher, setPayoutVoucher] = useState('')
  const [payoutStatus, setPayoutStatus] = useState('success')
  const typedAmount = refundUsd.trim()
  const typedReady = /^\d+(\.\d{1,8})?$/.test(typedAmount)
  useEffect(() => {
    setReason('')
    setRefundUsd('')
    setSelectedId('')
    setPayoutCurrency('')
    setPayoutAmount('')
    setPayoutAt('')
    setPayoutVoucher('')
    setPayoutStatus('success')
    setIdempotencyKey(crypto.randomUUID())
  }, [props.order])
  const preview = useQuery({
    queryKey: [
      'topup-refund-preview',
      props.order?.trade_no,
      typedReady ? typedAmount : '',
    ],
    queryFn: () =>
      previewTopupRefund(
        props.order?.trade_no ?? '',
        typedReady ? typedAmount : undefined
      ),
    enabled: props.order !== null && typedReady,
  })
  const listing = useQuery({
    queryKey: ['topup-manual-refunds', props.order?.trade_no],
    queryFn: () => listTopupManualRefunds(props.order?.trade_no ?? ''),
    enabled: props.order !== null,
  })
  const facts = listing.data?.facts ?? preview.data?.facts
  const occupancy = listing.data?.occupancy ?? facts?.occupancy
  const refunds = listing.data?.facts?.manual_refunds ?? facts?.manual_refunds ?? []
  const selected =
    refunds.find((row) => row.refund_id === selectedId) ?? refunds[0]
  useEffect(() => {
    if (!selectedId && refunds[0]?.refund_id) {
      setSelectedId(refunds[0].refund_id)
    }
  }, [refunds, selectedId])

  const refresh = () => {
    void listing.refetch()
    void preview.refetch()
    props.onDone()
  }

  const applyMutation = useMutation({
    mutationFn: async () => {
      const proof = await verification.requestVerification({
        scope: 'adty.topup.refund',
        context: {
          trade_no: props.order?.trade_no,
          reason: reason.trim(),
          refund_usd: typedAmount,
        },
        title: t('Confirm it is you'),
        description: t('Applying a refund requires a fresh verification.'),
      })
      if (!proof?.proof_token) throw new Error('cancelled')
      return refundTopup(
        props.order?.trade_no ?? '',
        reason.trim(),
        idempotencyKey,
        0,
        typedAmount,
        proof.proof_token
      )
    },
    onSuccess: (data) => {
      toast.success(t('Refund application submitted'))
      if (data.refund?.refund_id) setSelectedId(data.refund.refund_id)
      setIdempotencyKey(crypto.randomUUID())
      refresh()
    },
    onError: (error) => {
      if (error instanceof Error && error.message === 'cancelled') return
      handleServerError(error)
    },
  })

  const stepMutation = useMutation({
    mutationFn: async (step: 'approve' | 'reclaim' | 'bonus' | 'commission' | 'payout' | 'complete' | 'terminate') => {
      if (!selected) throw new Error('cancelled')
      const body: Record<string, unknown> = {
        idempotency_key: `${selected.refund_id}:${step}`,
      }
      if (step === 'payout') {
        body.status = payoutStatus
        body.currency = payoutCurrency.trim()
        body.amount = payoutAmount.trim()
        body.voucher = payoutVoucher.trim()
        body.paid_at = payoutAt.trim() ? Math.floor(new Date(payoutAt).getTime() / 1000) : 0
      }
      const proof = await verification.requestVerification({
        scope: 'adty.topup.manual_refund_step',
        context: {
          trade_no: props.order?.trade_no,
          refund_id: selected.refund_id,
          step,
          payout_status: step === 'payout' ? payoutStatus : undefined,
          payout_currency: step === 'payout' ? payoutCurrency.trim() : undefined,
          payout_amount: step === 'payout' ? payoutAmount.trim() : undefined,
          payout_voucher: step === 'payout' ? payoutVoucher.trim() : undefined,
          payout_at: step === 'payout' && payoutAt.trim() ? Math.floor(new Date(payoutAt).getTime() / 1000) : undefined,
        },
        title: t('Confirm it is you'),
        description: t('This refund step requires a fresh verification.'),
      })
      if (!proof?.proof_token) throw new Error('cancelled')
      return runTopupManualRefundStep(
        props.order?.trade_no ?? '',
        selected.refund_id,
        step,
        body,
        proof.proof_token
      )
    },
    onSuccess: (data, step) => {
      toast.success(
        step === 'complete' ? t('Refund completed') : t('Refund step recorded')
      )
      refresh()
    },
    onError: (error) => {
      if (error instanceof Error && error.message === 'cancelled') return
      handleServerError(error)
    },
  })

  const canApply =
    reason.trim() !== '' &&
    typedReady &&
    !applyMutation.isPending &&
    preview.data !== undefined
  const canStep = (ready: boolean) =>
    !!selected && ready && !stepMutation.isPending && selected.status !== 'completed' && selected.status !== 'terminated'

  return (
    <Dialog
      open={props.order !== null}
      onOpenChange={props.onOpenChange}
      title={t('Refund order')}
      description={t(
        'Apply a USD refund against the original top-up, then approve, record internal principal reclaim or the manual-handling shortfall, record the offline payout, and complete. Do not call the original payment channel.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
          <Button
            variant='destructive'
            onClick={() => applyMutation.mutate()}
            disabled={!canApply}
          >
            {t('Apply refund')}
          </Button>
        </>
      }
    >
      <SecureVerificationDialog {...verification.dialogProps} />
      {facts && (
        <dl className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm'>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Site ID')}</dt>
            <dd className='text-right tabular-nums'>{facts.site_id}</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Customer ID')}</dt>
            <dd className='text-right tabular-nums'>{facts.user_id}</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Original principal')}</dt>
            <dd className='text-right tabular-nums'>{facts.face_usd} USD</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Paid amount')}</dt>
            <dd className='text-right tabular-nums'>
              {facts.paid_amount} {facts.paid_currency}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Order rate')}</dt>
            <dd className='text-right tabular-nums'>{facts.order_rate}</dd>
          </div>
          {facts.principal_consumed_usd && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Consumed principal')}</dt>
              <dd className='text-right tabular-nums'>
                {facts.principal_consumed_usd} USD
              </dd>
            </div>
          )}
          {facts.principal_remaining_usd && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Remaining principal')}</dt>
              <dd className='text-right tabular-nums'>
                {facts.principal_remaining_usd} USD
              </dd>
            </div>
          )}
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('In-flight precharge')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(facts.pending_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Completed refund (S)')}</dt>
            <dd className='text-right tabular-nums'>
              {occupancy?.completed_usd ?? facts.refund_completed_usd ?? facts.cumulative_refund_usd} USD
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('In-progress refund (H)')}</dt>
            <dd className='text-right tabular-nums'>
              {occupancy?.in_progress_usd ?? facts.refund_in_progress_usd ?? '0'} USD
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Unknown payout (U)')}</dt>
            <dd className='text-right tabular-nums'>
              {occupancy?.unknown_usd ?? facts.refund_unknown_usd ?? '0'} USD
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Refund occupancy')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(occupancy?.occupied_quota ?? facts.refund_occupied_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Remaining refund cap')}</dt>
            <dd className='text-right tabular-nums'>
              {occupancy?.remaining_usd ?? facts.refund_remaining_usd ?? ''} USD
            </dd>
          </div>
          {(facts.manual_shortfall_usd || facts.manual_shortfall_quota) && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Manual-handling shortfall')}</dt>
              <dd className='text-right tabular-nums'>
                {facts.manual_shortfall_usd} USD
              </dd>
            </div>
          )}
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Bonus')}</dt>
            <dd className='text-right tabular-nums'>{facts.bonus_usd} USD</dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Bonus used')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(facts.bonus_used_quota)}
            </dd>
          </div>
          <div className='contents'>
            <dt className='text-muted-foreground'>{t('Bonus remaining')}</dt>
            <dd className='text-right tabular-nums'>
              {formatQuota(facts.bonus_remaining_quota)}
            </dd>
          </div>
          {facts.foreign_reference && (
            <div className='contents'>
              <dt className='text-muted-foreground'>{t('Foreign reference')}</dt>
              <dd className='text-right tabular-nums'>
                {facts.foreign_reference} {facts.paid_currency}
              </dd>
            </div>
          )}
        </dl>
      )}
      <div className='space-y-1'>
        <Label htmlFor='refund-usd'>{t('Refund amount in USD')}</Label>
        <Input
          id='refund-usd'
          value={refundUsd}
          inputMode='decimal'
          autoComplete='off'
          onChange={(event) => setRefundUsd(event.target.value)}
        />
      </div>
      {preview.data && (
        <CommissionRefundPreviewDetails preview={preview.data} t={t} />
      )}
      <div className='space-y-2'>
        <Label htmlFor='refund-reason'>{t('Reason (required)')}</Label>
        <Textarea
          id='refund-reason'
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={3}
        />
      </div>
      {refunds.length > 0 && (
        <div className='space-y-2'>
          <Label htmlFor='refund-select'>{t('Refund applications')}</Label>
          <select
            id='refund-select'
            className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
            value={selected?.refund_id ?? ''}
            onChange={(event) => setSelectedId(event.target.value)}
          >
            {refunds.map((row) => (
              <option key={row.refund_id} value={row.refund_id}>
                {row.refund_id} · {row.request_usd} USD · {row.status}
                {row.shortfall_quota ? ` · ${t('shortfall')} ${row.shortfall_usd}` : ''}
              </option>
            ))}
          </select>
          {selected && (
            <p className='text-muted-foreground text-sm'>
              {t('Internal reclaim')}: {selected.reclaimed_at ? t('recorded') : t('pending')}. {t('Offline payout')}: {selected.payout_status || t('pending')}. {t('Gift')}: {selected.bonus_at ? t('recorded') : t('pending')}. {t('Commission')}: {selected.commission_at ? t('recorded') : t('pending')}.
            </p>
          )}
          {selected && selected.status !== 'completed' && selected.status !== 'terminated' && (
            <div className='flex flex-wrap gap-2'>
              <Button
                variant='outline'
                disabled={!canStep(!selected.approved_at)}
                onClick={() => stepMutation.mutate('approve')}
              >
                {t('Approve')}
              </Button>
              <Button
                variant='outline'
                disabled={!canStep(!!selected.approved_at && !selected.reclaimed_at)}
                onClick={() => stepMutation.mutate('reclaim')}
              >
                {t('Record internal reclaim')}
              </Button>
              <Button
                variant='outline'
                disabled={!canStep(!!selected.approved_at && !selected.bonus_at)}
                onClick={() => stepMutation.mutate('bonus')}
              >
                {t('Record gift reclaim')}
              </Button>
              <Button
                variant='outline'
                disabled={!canStep(!!selected.approved_at && !selected.commission_at)}
                onClick={() => stepMutation.mutate('commission')}
              >
                {t('Record commission handling')}
              </Button>
              <Button
                variant='outline'
                disabled={!canStep(!!selected.approved_at && !!selected.reclaimed_at && !!selected.bonus_at && !!selected.commission_at && selected.payout_status === 'success')}
                onClick={() => stepMutation.mutate('complete')}
              >
                {t('Complete refund')}
              </Button>
            </div>
          )}
          {selected && selected.status !== 'completed' && selected.status !== 'terminated' && !!selected.approved_at && (
            <div className='space-y-2'>
              <Label htmlFor='payout-currency'>{t('Offline payout currency')}</Label>
              <Input
                id='payout-currency'
                value={payoutCurrency}
                onChange={(event) => setPayoutCurrency(event.target.value)}
              />
              <Label htmlFor='payout-amount'>{t('Offline payout amount')}</Label>
              <Input
                id='payout-amount'
                value={payoutAmount}
                inputMode='decimal'
                onChange={(event) => setPayoutAmount(event.target.value)}
              />
              <Label htmlFor='payout-at'>{t('Offline payout time')}</Label>
              <Input
                id='payout-at'
                type='datetime-local'
                value={payoutAt}
                onChange={(event) => setPayoutAt(event.target.value)}
              />
              <Label htmlFor='payout-voucher'>{t('Offline payout voucher')}</Label>
              <Input
                id='payout-voucher'
                value={payoutVoucher}
                onChange={(event) => setPayoutVoucher(event.target.value)}
              />
              <Label htmlFor='payout-status'>{t('Offline payout status')}</Label>
              <select
                id='payout-status'
                className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
                value={payoutStatus}
                onChange={(event) => setPayoutStatus(event.target.value)}
              >
                <option value='success'>{t('Success')}</option>
                <option value='unknown'>{t('Unknown')}</option>
                <option value='failed'>{t('Failed')}</option>
              </select>
              <Button
                variant='outline'
                disabled={!canStep(!!selected.approved_at)}
                onClick={() => stepMutation.mutate('payout')}
              >
                {t('Record offline payout')}
              </Button>
            </div>
          )}
        </div>
      )}
    </Dialog>
  )
}

function isPositiveAmount(value?: string) {
  return !!value && value !== '0'
}

function CommissionRefundPreviewDetails(props: {
  preview: TopupRefundResult
  t: (key: string) => string
}) {
  const commission = props.preview.commission ?? props.preview.facts?.commission
  const t = props.t
  if (!commission && !props.preview.commission_issued_usd) {
    return null
  }
  const expected = commission
    ? [
        [t('Expected release'), commission.expected_release_usd],
        [t('Expected clawback from balance'), commission.expected_converted_clawback_usd],
        [t('Expected recovery'), commission.expected_withdrawn_recovery_usd],
        [
          t('Expected pending-coverage reduction'),
          commission.expected_pending_coverage_cancel_usd,
        ],
      ]
    : []
  return (
    <div className='space-y-1 text-sm'>
      {(commission?.issued_usd || props.preview.commission_issued_usd) && (
        <p className='tabular-nums'>
          {t('Issued commission')}:{' '}
          {commission?.issued_usd ?? props.preview.commission_issued_usd} USD
        </p>
      )}
      {commission?.states?.map((item) => (
        <p key={item.state} className='tabular-nums'>
          {t(officialCommissionStateLabel(item.state))}: {item.amount_usd} USD
        </p>
      ))}
      {expected.map(([label, amount]) => (
        <p key={label} className='tabular-nums'>
          {label}: {amount} USD
        </p>
      ))}
      {commission?.withdrawn_recovery_liable === 'inviter' &&
        isPositiveAmount(commission.expected_withdrawn_recovery_usd) && (
          <p>{t('Recovery liable party: inviter')}</p>
        )}
    </div>
  )
}

