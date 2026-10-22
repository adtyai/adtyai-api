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
import { Info, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
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
import { getServerErrorMessage } from '@/lib/server-error-message'
import { transferAffiliateQuota } from '@/features/wallet/api'
import {
  USDT_CHAIN_CATALOG,
  usdtChainById,
  validateUsdtPayoutAddress,
} from '@/lib/usdt-chains'
import { cn } from '@/lib/utils'

import { fetchAffPayoutQrObjectUrl, uploadAffPayoutQr } from './api'
import {
  useAffCommissionsSelf,
  useAffSelf,
  useAffWithdrawalsSelf,
  useCreateAffWithdraw,
} from './hooks/use-affiliate'
import {
  affQuotaToUsd,
  affUsdToQuota,
  estimateWithdrawFeeQuota,
  formatAffRate,
  formatAffUsd,
  newWithdrawIdemKey,
} from './lib/quota'
import {
  commissionStatusLabel,
  withdrawMethodLabel,
  withdrawStatusLabel,
} from './lib/status'
import type { AffCommissionStatus } from './types'

function statusTone(status: string): string {
  switch (status) {
    case 'frozen':
    case 'pending':
    case 'approved':
      return 'bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-200'
    case 'available':
    case 'paid':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-200'
    case 'voided':
    case 'rejected':
    case 'pay_failed':
    case 'pay_failed_final':
      return 'bg-rose-50 text-rose-700 ring-rose-200 dark:bg-rose-950/40 dark:text-rose-200'
    case 'blocked':
    case 'pay_unknown':
      return 'bg-violet-50 text-violet-700 ring-violet-200 dark:bg-violet-950/40 dark:text-violet-200'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

export function AffiliateWithdrawPage() {
  const { t } = useTranslation()
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [commissionOffset, setCommissionOffset] = useState(0)
  const [withdrawalOffset, setWithdrawalOffset] = useState(0)
  const pageSize = 50
  const self = useAffSelf()
  const commissions = useAffCommissionsSelf({
    limit: pageSize,
    offset: commissionOffset,
    status: statusFilter === 'all' ? undefined : statusFilter,
  })
  const withdrawals = useAffWithdrawalsSelf({
    limit: pageSize,
    offset: withdrawalOffset,
  })
  const createWd = useCreateAffWithdraw()
  const [amountUsd, setAmountUsd] = useState('')
  const [method, setMethod] = useState<'balance' | 'usdt' | 'alipay'>('usdt')
  const [payAccount, setPayAccount] = useState('')
  const [alipayAccount, setAlipayAccount] = useState('')
  const [payoutQrId, setPayoutQrId] = useState(0)
  const [payoutQrName, setPayoutQrName] = useState('')
  const [qrPreview, setQrPreview] = useState<string | null>(null)
  const [uploadingQr, setUploadingQr] = useState(false)
  const [usdtChain, setUsdtChain] = useState('')
  const [confirmedNetwork, setConfirmedNetwork] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)
  const [idemKey, setIdemKey] = useState(() => newWithdrawIdemKey())
  const [transferring, setTransferring] = useState(false)
  const submitLock = useRef(false)

  const data = self.data
  const amountNum = Number(amountUsd)
  const amountQuota = useMemo(
    () =>
      Number.isFinite(amountNum) && amountNum > 0
        ? affUsdToQuota(amountNum)
        : 0,
    [amountNum]
  )
  const feePreview = useMemo(() => {
    if (method === 'balance') {
      return { feeQuota: 0, netQuota: amountQuota }
    }
    const rate = data?.withdraw_fee_rate ?? 0
    return estimateWithdrawFeeQuota(amountQuota, rate)
  }, [amountQuota, data?.withdraw_fee_rate, method])
  let arrivalNote = t(
    'USDT is paid manually after review. Submitting freezes invite earnings.'
  )
  if (method === 'balance') {
    arrivalNote = t(
      'Balance transfer arrives immediately in paid balance. No fee and no withdrawal review.'
    )
  } else if (method === 'alipay') {
    arrivalNote = t(
      'Alipay is paid manually after review. This site does not call Alipay. Submitting freezes invite earnings.'
    )
  }

  const commissionRows = commissions.data?.rows ?? []

  useEffect(() => {
    return () => {
      if (qrPreview) URL.revokeObjectURL(qrPreview)
    }
  }, [qrPreview])

  async function handleSubmit() {
    if (submitLock.current) return
    submitLock.current = true
    setLocalError(null)
    try {
    if (!data) return
    if (!Number.isFinite(amountNum) || amountNum <= 0) {
      setLocalError(t('Enter a valid withdraw amount'))
      return
    }
    if (
      method !== 'balance' &&
      amountNum + 1e-9 < Number(data.min_withdraw_usd || 0)
    ) {
      setLocalError(
        `${t('Below minimum withdraw')} ($${Number(
          data.min_withdraw_usd
        ).toFixed(2)})`
      )
      return
    }
    if (amountQuota > (data.aff_quota || 0)) {
      setLocalError(t('Insufficient affiliate quota'))
      return
    }
    if (method === 'balance') {
      setTransferring(true)
      try {
        const res = await transferAffiliateQuota({ quota: amountQuota })
        if (!res.success) {
          const msg = getServerErrorMessage(res, t('Transfer failed'))
          setLocalError(msg)
          toast.error(msg)
          return
        }
        toast.success(t('Transfer successful'))
        setAmountUsd('')
        void self.refetch()
      } catch (err) {
        const msg = getServerErrorMessage(err, t('Transfer failed'))
        setLocalError(msg)
        toast.error(msg)
      } finally {
        setTransferring(false)
      }
      return
    }
    let account = ''
    let chainId = ''
    if (method === 'usdt') {
      const chain = usdtChainById(usdtChain)
      if (!chain) {
        setLocalError(t('Select a payout network'))
        return
      }
      account = payAccount.trim()
      if (validateUsdtPayoutAddress(chain.id, account)) {
        setLocalError(t('USDT address does not match the selected network'))
        return
      }
      if (!confirmedNetwork) {
        setLocalError(t('Confirm the payout network before submitting'))
        return
      }
      chainId = chain.id
    } else {
      account = alipayAccount.trim()
      if (account.length < 6 || /\s|[/:]/.test(account)) {
        setLocalError(t('Alipay account'))
        return
      }
      if (payoutQrId <= 0) {
        setLocalError(t('Upload payout QR'))
        return
      }
    }
    try {
      const res = await createWd.mutateAsync({
        amount_quota: amountQuota,
        idem_key: idemKey,
        pay_method: method,
        pay_account: account,
        usdt_chain: chainId,
        payout_qr_id: method === 'alipay' ? payoutQrId : undefined,
      })
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Withdraw failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Withdraw submitted'))
      setAmountUsd('')
      setPayoutQrId(0)
      setPayoutQrName('')
      setQrPreview(null)
      setIdemKey(newWithdrawIdemKey())
      void self.refetch()
      void withdrawals.refetch()
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Withdraw failed'))
      setLocalError(msg)
      toast.error(msg)
    }
    } finally {
      submitLock.current = false
    }
  }

  async function handleQrFile(file: File | undefined) {
    setLocalError(null)
    if (!file) return
    if (file.size > 512 * 1024) {
      setLocalError(t('Upload payout QR'))
      return
    }
    setUploadingQr(true)
    try {
      const res = await uploadAffPayoutQr(file)
      if (!res.success || !res.data?.id) {
        const msg = getServerErrorMessage(res, t('Upload payout QR'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      setPayoutQrId(res.data.id)
      setPayoutQrName(file.name)
      try {
        setQrPreview(await fetchAffPayoutQrObjectUrl(res.data.id))
      } catch (previewErr) {
        setQrPreview(null)
        toast.error(
          getServerErrorMessage(previewErr, t('Failed to load payout QR'))
        )
      }
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Upload payout QR'))
      setLocalError(msg)
      toast.error(msg)
    } finally {
      setUploadingQr(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Commissions & withdraw')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          className='h-8'
          render={<Link to='/affiliate' />}
        >
          {t('Invite & rewards')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => {
            void self.refetch()
            void commissions.refetch()
            void withdrawals.refetch()
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
              'Commission is based on invitee paid amount. After unfreeze you can withdraw. Wallet balance (including gifts) cannot be withdrawn.'
            )}
          </p>

          {(self.isError || commissions.isError) && !self.data ? (
            <ErrorState
              title={t('Failed to load affiliate data')}
              onRetry={() => {
                void self.refetch()
                void commissions.refetch()
              }}
            />
          ) : null}

          <div className='grid gap-4 lg:grid-cols-[1fr_360px]'>
            <div className='space-y-4'>
              <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                <div className='flex flex-wrap items-center justify-between gap-2 border-b px-5 pt-4 pb-3'>
                  <div className='text-[15px] font-semibold'>
                    {t('Commission details')}
                  </div>
                  <div className='bg-muted inline-flex rounded-lg p-1 text-[13px]'>
                    {(
                      [
                        ['all', t('All')],
                        ['frozen', t('Frozen')],
                        ['available', t('Available')],
                        ['voided', t('Voided')],
                        ['blocked', t('Blocked')],
                      ] as [string, string][]
                    ).map(([id, label]) => (
                      <button
                        key={id}
                        type='button'
                        className={cn(
                          'rounded-md px-3 py-1.5',
                          statusFilter === id
                            ? 'bg-background text-foreground font-medium shadow-sm'
                            : 'text-muted-foreground'
                        )}
                        onClick={() => {
                          setStatusFilter(id)
                          setCommissionOffset(0)
                        }}
                      >
                        {label}
                      </button>
                    ))}
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow className='bg-muted/40 hover:bg-muted/40'>
                      <TableHead>{t('Time')}</TableHead>
                      <TableHead>{t('Invitee')}</TableHead>
                      <TableHead>{t('Order')}</TableHead>
                      <TableHead>{t('Paid base')}</TableHead>
                      <TableHead>{t('Rate')}</TableHead>
                      <TableHead>{t('Commission')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {commissions.isLoading ? (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-muted-foreground h-20 text-center text-sm'
                        >
                          {t('Loading...')}
                        </TableCell>
                      </TableRow>
                    ) : null}
                    {!commissions.isLoading &&
                      (commissionRows.length === 0 ? (
                        <TableRow>
                          <TableCell
                            colSpan={7}
                            className='text-muted-foreground h-20 text-center text-sm'
                          >
                            {t('No commissions')}
                          </TableCell>
                        </TableRow>
                      ) : (
                        commissionRows.map((row) => (
                          <TableRow key={row.id}>
                            <TableCell className='text-xs whitespace-nowrap'>
                              {row.created_at
                                ? new Date(row.created_at).toLocaleString()
                                : '-'}
                            </TableCell>
                            <TableCell>
                              {row.invitee_name || row.invitee_id}
                            </TableCell>
                            <TableCell>
                              <span className='font-mono text-[12px]'>
                                {row.source} {row.order_no}
                              </span>
                            </TableCell>
                            <TableCell>
                              ${Number(row.paid_amount_usd || 0).toFixed(2)}
                            </TableCell>
                            <TableCell>
                              {formatAffRate(row.rate_snapshot)}
                            </TableCell>
                            <TableCell className='font-semibold'>
                              {formatAffUsd(row.commission_quota)}
                            </TableCell>
                            <TableCell>
                              <span
                                className={cn(
                                  'inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium ring-1 ring-inset',
                                  statusTone(row.status)
                                )}
                              >
                                {t(
                                  commissionStatusLabel(
                                    row.status as AffCommissionStatus
                                  )
                                )}
                                {row.status === 'frozen' && row.unfreeze_at
                                  ? ` · ${new Date(
                                      row.unfreeze_at
                                    ).toLocaleDateString()}`
                                  : ''}
                              </span>
                            </TableCell>
                          </TableRow>
                        ))
                      ))}
                  </TableBody>
                </Table>
                {(commissions.data?.total ?? 0) > pageSize ? (
                  <div className='flex justify-end gap-2 border-t px-5 py-3'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7'
                      disabled={commissionOffset <= 0}
                      onClick={() =>
                        setCommissionOffset(
                          Math.max(0, commissionOffset - pageSize)
                        )
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
                        commissionOffset + pageSize >=
                        (commissions.data?.total ?? 0)
                      }
                      onClick={() =>
                        setCommissionOffset(commissionOffset + pageSize)
                      }
                    >
                      {t('Next')}
                    </Button>
                  </div>
                ) : null}
              </section>

              <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                <div className='border-b px-5 pt-4 pb-3'>
                  <div className='text-[15px] font-semibold'>
                    {t('Withdraw history')}
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow className='bg-muted/40 hover:bg-muted/40'>
                      <TableHead>ID</TableHead>
                      <TableHead>{t('Amount')}</TableHead>
                      <TableHead>{t('Fee')}</TableHead>
                      <TableHead>{t('Payout method')}</TableHead>
                      <TableHead>{t('Payout account')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Created at')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(withdrawals.data?.rows ?? []).length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-muted-foreground h-20 text-center text-sm'
                        >
                          {t('No withdrawals')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      (withdrawals.data?.rows ?? []).map((row) => (
                        <TableRow key={row.id}>
                          <TableCell>
                            <code className='text-xs'>{row.id}</code>
                          </TableCell>
                          <TableCell className='font-semibold'>
                            {formatAffUsd(row.amount_quota)}
                          </TableCell>
                          <TableCell>{formatAffUsd(row.fee_quota)}</TableCell>
                          <TableCell>
                            {t(withdrawMethodLabel(row))}
                            {row.usdt_chain
                              ? ` · ${usdtChainById(row.usdt_chain)?.label || row.usdt_chain}`
                              : ''}
                          </TableCell>
                          <TableCell>
                            <span className='font-mono text-[12px] break-all'>
                              {row.pay_account || '-'}
                            </span>
                            {row.payout_ref ? (
                              <div className='text-muted-foreground text-[11px]'>
                                {row.payout_ref}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant='secondary'
                              className={cn(
                                'font-normal ring-1 ring-inset',
                                statusTone(row.status)
                              )}
                            >
                              {t(withdrawStatusLabel(row.status))}
                            </Badge>
                          </TableCell>
                          <TableCell className='text-xs whitespace-nowrap'>
                            {row.created_at
                              ? new Date(row.created_at).toLocaleString()
                              : '-'}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
                {(withdrawals.data?.total ?? 0) > pageSize ? (
                  <div className='flex justify-end gap-2 border-t px-5 py-3'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7'
                      disabled={withdrawalOffset <= 0}
                      onClick={() =>
                        setWithdrawalOffset(
                          Math.max(0, withdrawalOffset - pageSize)
                        )
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
                        withdrawalOffset + pageSize >=
                        (withdrawals.data?.total ?? 0)
                      }
                      onClick={() =>
                        setWithdrawalOffset(withdrawalOffset + pageSize)
                      }
                    >
                      {t('Next')}
                    </Button>
                  </div>
                ) : null}
              </section>
            </div>

            <section className='bg-card h-fit overflow-hidden rounded-xl border shadow-xs'>
              <div className='border-b px-5 pt-4 pb-3'>
                <div className='text-[15px] font-semibold'>
                  {t('Request withdraw')}
                </div>
              </div>
              <div className='space-y-4 p-5'>
                <div className='bg-primary/5 border-primary/20 rounded-xl border p-4'>
                  <div className='text-primary text-xs'>
                    {t('Available to withdraw')}
                  </div>
                  <div className='text-primary mt-1 text-[28px] font-semibold'>
                    {formatAffUsd(data?.aff_quota ?? 0)}
                  </div>
                  <div className='text-muted-foreground mt-1 text-xs'>
                    {t('Frozen')}: {formatAffUsd(data?.frozen_quota ?? 0)}
                  </div>
                </div>

                <div>
                  <div className='mb-1.5 text-[13px] font-medium'>
                    {t('Withdraw amount')} (USD)
                  </div>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    value={amountUsd}
                    onChange={(e) => setAmountUsd(e.target.value)}
                    placeholder='0.00'
                  />
                  <div className='text-muted-foreground mt-1 text-[11.5px]'>
                    {method === 'balance'
                      ? t(
                          'Balance transfer arrives immediately in paid balance. No fee and no withdrawal review.'
                        )
                      : `${t('Minimum withdraw')}: $${Number(
                          data?.min_withdraw_usd || 0
                        ).toFixed(2)} · ${t('Withdraw fee rate')}: ${
                          formatAffRate(data?.withdraw_fee_rate)
                        }`}
                  </div>
                </div>

                <div>
                  <div className='mb-1.5 text-[13px] font-medium'>
                    {t('Payout method')}
                  </div>
                  <Select
                    items={[
                      { value: 'balance', label: t('Transfer to balance') },
                      { value: 'usdt', label: 'USDT' },
                      { value: 'alipay', label: t('Alipay') },
                    ]}
                    value={method}
                    onValueChange={(value) => {
                      if (
                        value === 'balance' ||
                        value === 'usdt' ||
                        value === 'alipay'
                      ) {
                        setMethod(value)
                        setIdemKey(newWithdrawIdemKey())
                        setLocalError(null)
                      }
                    }}
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='balance'>
                        {t('Transfer to balance')}
                      </SelectItem>
                      <SelectItem value='usdt'>USDT</SelectItem>
                      <SelectItem value='alipay'>{t('Alipay')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <div className='text-muted-foreground mt-1 text-[11.5px]'>
                    {arrivalNote}
                  </div>
                </div>

                {method === 'usdt' ? (
                <>
                <div>
                  <div className='mb-1.5 text-[13px] font-medium'>
                    {t('Payout network')}
                  </div>
                  <Select
                    items={USDT_CHAIN_CATALOG.map((chain) => ({
                      value: chain.id,
                      label: chain.label,
                    }))}
                    value={usdtChain}
                    onValueChange={(value) => {
                      setUsdtChain(value || '')
                      setConfirmedNetwork(false)
                    }}
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue placeholder={t('Select a payout network')} />
                    </SelectTrigger>
                    <SelectContent>
                      {USDT_CHAIN_CATALOG.map((chain) => (
                        <SelectItem key={chain.id} value={chain.id}>
                          {chain.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div>
                  <div className='mb-1.5 text-[13px] font-medium'>
                    {t('USDT address')}
                  </div>
                  <Input
                    value={payAccount}
                    onChange={(e) => {
                      setPayAccount(e.target.value)
                      setConfirmedNetwork(false)
                    }}
                    placeholder={t('USDT address')}
                    className='font-mono text-[12.5px]'
                  />
                </div>

                <label className='flex items-start gap-2 text-[12.5px] leading-5'>
                  <Checkbox
                    className='mt-0.5'
                    checked={confirmedNetwork}
                    onCheckedChange={(value) =>
                      setConfirmedNetwork(value === true)
                    }
                  />
                  <span>
                    {t(
                      'Confirm this address is on {{network}}. A wrong network can lose the payout.',
                      {
                        network:
                          usdtChainById(usdtChain)?.network ||
                          t('Select a payout network'),
                      }
                    )}
                  </span>
                </label>
                </>
                ) : null}

                {method === 'alipay' ? (
                  <div className='space-y-3'>
                    <div>
                      <div className='mb-1.5 text-[13px] font-medium'>
                        {t('Alipay account')}
                      </div>
                      <Input
                        value={alipayAccount}
                        onChange={(e) => setAlipayAccount(e.target.value)}
                        placeholder={t('Alipay account')}
                        className='font-mono text-[12.5px]'
                      />
                    </div>
                    <div>
                      <div className='mb-1.5 text-[13px] font-medium'>
                        {t('Payout QR')}
                      </div>
                      <Input
                        type='file'
                        accept='image/png,image/jpeg,image/webp'
                        onChange={(e) =>
                          void handleQrFile(e.target.files?.[0])
                        }
                      />
                      <div className='text-muted-foreground mt-1 text-[11.5px]'>
                        {payoutQrId > 0
                          ? payoutQrName || String(payoutQrId)
                          : t('Upload payout QR')}
                      </div>
                      {qrPreview ? (
                        <img
                          src={qrPreview}
                          alt={t('Payout QR')}
                          className='mt-2 h-28 w-28 rounded border object-contain'
                        />
                      ) : null}
                    </div>
                  </div>
                ) : null}

                <div className='space-y-2 text-[13px]'>
                  <div className='flex justify-between gap-3 border-b border-dashed pb-2'>
                    <span className='text-muted-foreground'>{t('Fee')}</span>
                    <span className='font-medium'>
                      {formatAffUsd(feePreview.feeQuota)}
                    </span>
                  </div>
                  <div className='flex justify-between gap-3 border-b border-dashed pb-2'>
                    <span className='text-muted-foreground'>
                      {t('Net amount')}
                    </span>
                    <span className='font-medium'>
                      {formatAffUsd(feePreview.netQuota)}
                    </span>
                  </div>
                  <div className='text-muted-foreground text-[11px]'>
                    {t('≈ {{usd}} USD → {{quota}} quota', {
                      usd: affQuotaToUsd(amountQuota).toFixed(2),
                      quota: amountQuota || 0,
                    })}
                  </div>
                </div>

                {localError ? (
                  <div className='text-destructive border-destructive/30 bg-destructive/5 rounded-lg border px-3 py-2 text-xs'>
                    {localError}
                  </div>
                ) : null}

                <Button
                  type='button'
                  className='w-full'
                  disabled={
                    createWd.isPending ||
                    uploadingQr ||
                    transferring ||
                    !data
                  }
                  onClick={() => void handleSubmit()}
                >
                  {t('Submit withdraw')}
                </Button>

                <div className='bg-muted/40 text-muted-foreground flex gap-2.5 rounded-lg border px-3.5 py-2.5 text-[12px] leading-5'>
                  <Info className='mt-0.5 size-4 shrink-0' />
                  <div>
                    {t(
                      'Only affiliate commission can be withdrawn. Wallet balance (including gifts) cannot be withdrawn.'
                    )}
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export default AffiliateWithdrawPage
