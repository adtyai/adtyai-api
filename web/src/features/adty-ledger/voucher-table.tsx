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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatTimestamp } from '@/lib/format'

import {
  voucherCanCancel,
  voucherCanFreeze,
  voucherPurposeLabel,
  voucherSourceLabel,
  voucherStatusLabel,
  type Voucher,
} from './voucher-labels'

type PendingAction = { kind: 'freeze' | 'cancel'; voucher: Voucher }

/**
 * Issued recharge codes with freeze and cancel. A frozen code cannot be
 * redeemed and has no unfreeze; cancelling releases the amount back to its
 * source. Both ask for confirmation first.
 */
export function VoucherTable(props: {
  label: string
  rows: Voucher[]
  busy: boolean
  onFreeze: (voucher: Voucher) => void
  onCancel: (voucher: Voucher) => void
  showIssuer?: boolean
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<PendingAction | null>(null)
  const statusVariant = (status: string) => {
    if (status === 'redeemed') return 'success' as const
    if (status === 'frozen') return 'warning' as const
    return 'neutral' as const
  }
  return (
    <>
      <StaticDataTable
        tableClassName='min-w-[820px]'
        tableProps={{ 'aria-label': props.label }}
        data={props.rows}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState className='min-h-24' title={t('No recharge codes')} />
        }
        columns={[
          {
            id: 'code',
            header: t('Voucher code'),
            cell: (row) => (
              <code className='font-mono text-xs'>…{row.code_tail}</code>
            ),
          },
          {
            id: 'purpose',
            header: t('Purpose'),
            cell: (row) => voucherPurposeLabel(t, row.purpose),
          },
          {
            id: 'amount',
            header: t('Amount (USD)'),
            cellClassName: 'tabular-nums',
            cell: (row) => row.amount_usd,
          },
          {
            id: 'source',
            header: t('Funded from'),
            cell: (row) => voucherSourceLabel(t, row.source_kind),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <StatusBadge
                label={voucherStatusLabel(t, row.status)}
                variant={statusVariant(row.status)}
                copyable={false}
              />
            ),
          },
          ...(props.showIssuer
            ? [
                {
                  id: 'issuer',
                  header: t('Issued by'),
                  cell: (row: Voucher) => `#${row.issuer_id}`,
                },
              ]
            : []),
          {
            id: 'time',
            header: t('Issued at'),
            cell: (row) => formatTimestamp(row.created_at),
          },
          {
            id: 'redeemed',
            header: t('Redeemed by'),
            cell: (row) =>
              row.redeemer_id > 0
                ? `#${row.redeemer_id} · ${formatTimestamp(row.redeemed_at)}`
                : '—',
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (row) =>
              voucherCanCancel(row) ? (
                <div className='flex justify-end gap-1'>
                  {voucherCanFreeze(row) && (
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={props.busy}
                      onClick={() =>
                        setPending({ kind: 'freeze', voucher: row })
                      }
                    >
                      {t('Freeze')}
                    </Button>
                  )}
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={props.busy}
                    onClick={() => setPending({ kind: 'cancel', voucher: row })}
                  >
                    {t('Cancel code')}
                  </Button>
                </div>
              ) : null,
          },
        ]}
      />
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null)
        }}
        title={
          pending?.kind === 'freeze'
            ? t('Freeze this code?')
            : t('Cancel this code?')
        }
        desc={
          pending?.kind === 'freeze'
            ? t(
                'A frozen code cannot be redeemed and cannot be unfrozen. The amount stays held until the code is cancelled.'
              )
            : t(
                'The code can no longer be redeemed and its amount returns to where it was funded from.'
              )
        }
        confirmText={
          pending?.kind === 'freeze' ? t('Freeze') : t('Cancel code')
        }
        destructive
        handleConfirm={() => {
          if (!pending) return
          if (pending.kind === 'freeze') props.onFreeze(pending.voucher)
          else props.onCancel(pending.voucher)
          setPending(null)
        }}
      />
    </>
  )
}
