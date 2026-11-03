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
import { Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { safeJsonParseWithValidation } from '../utils/json-parser'
import { isObjectRecord } from '../utils/json-validators'

type BonusTier = { amount: number; rate: number }

type AdtyBonusRatesEditorProps = {
  value: string
  onChange: (value: string) => void
}

function parseTiers(value: string): BonusTier[] {
  const parsed = safeJsonParseWithValidation<Record<string, unknown>>(value, {
    fallback: {},
    validator: isObjectRecord,
    silent: true,
  })
  return Object.entries(parsed)
    .map(([amount, rate]) => ({
      amount: Number.parseInt(amount, 10),
      rate: typeof rate === 'number' ? rate : Number.parseFloat(String(rate)),
    }))
    .filter(
      (tier) => Number.isFinite(tier.amount) && Number.isFinite(tier.rate)
    )
    .sort((a, b) => a.amount - b.amount)
}

function serializeTiers(tiers: BonusTier[]): string {
  const out: Record<string, number> = {}
  for (const tier of tiers) out[String(tier.amount)] = tier.rate
  return JSON.stringify(out)
}

/** Bonus percent by top-up tier (adty task 4.3); saved as {"10":1,"30":1.5,…}. */
export function AdtyBonusRatesEditor(props: AdtyBonusRatesEditorProps) {
  const { t } = useTranslation()
  const tiers = useMemo(() => parseTiers(props.value), [props.value])
  const [editing, setEditing] = useState<BonusTier | null>(null)
  const [open, setOpen] = useState(false)
  const [amount, setAmount] = useState('')
  const [rate, setRate] = useState('')

  const amountValue = Number.parseInt(amount, 10)
  const rateValue = Number.parseFloat(rate)
  const amountError =
    !Number.isInteger(amountValue) || amountValue <= 0
      ? t('Amount must be a whole number greater than 0')
      : ''
  const rateError =
    !Number.isFinite(rateValue) || rateValue <= 0 || rateValue > 100
      ? t('Bonus must be greater than 0 and at most 100')
      : ''

  const openDialog = (tier: BonusTier | null) => {
    setEditing(tier)
    setAmount(tier ? String(tier.amount) : '')
    setRate(tier ? String(tier.rate) : '')
    setOpen(true)
  }

  const save = () => {
    if (amountError || rateError) return
    const next = tiers.filter(
      (tier) => tier.amount !== amountValue && tier.amount !== editing?.amount
    )
    next.push({ amount: amountValue, rate: rateValue })
    props.onChange(serializeTiers(next))
    setOpen(false)
  }

  return (
    <div className='space-y-3'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <p className='text-muted-foreground text-sm'>
          {t('The bonus of the largest tier not above the amount applies.')}
        </p>
        <Button type='button' size='sm' onClick={() => openDialog(null)}>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add bonus tier')}
        </Button>
      </div>
      <StaticDataTable
        tableProps={{ 'aria-label': t('Bonus tiers') }}
        data={tiers}
        getRowKey={(tier) => String(tier.amount)}
        emptyContent={
          <span className='text-muted-foreground text-sm'>
            {t('No bonus tiers configured')}
          </span>
        }
        columns={[
          {
            id: 'amount',
            header: t('Recharge Amount'),
            cell: (tier) => (
              <span className='font-mono text-sm'>${tier.amount}</span>
            ),
          },
          {
            id: 'rate',
            header: t('Bonus %'),
            cell: (tier) => (
              <span className='font-mono text-sm'>{tier.rate}%</span>
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (tier) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => openDialog(tier)}
                onDelete={() =>
                  props.onChange(
                    serializeTiers(
                      tiers.filter((item) => item.amount !== tier.amount)
                    )
                  )
                }
              />
            ),
          },
        ]}
      />
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={editing ? t('Edit bonus tier') : t('Add bonus tier')}
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={save}
              disabled={Boolean(amountError || rateError)}
            >
              {editing ? t('Update') : t('Add')}
            </Button>
          </>
        }
      >
        <div className='space-y-2'>
          <Label htmlFor='bonus-tier-amount'>
            {t('Recharge Amount (USD)')}
          </Label>
          <Input
            id='bonus-tier-amount'
            type='number'
            min={1}
            step={1}
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            aria-invalid={amount !== '' && Boolean(amountError)}
          />
          {amount !== '' && amountError && (
            <p className='text-destructive text-xs'>{amountError}</p>
          )}
        </div>
        <div className='space-y-2'>
          <Label htmlFor='bonus-tier-rate'>{t('Bonus %')}</Label>
          <Input
            id='bonus-tier-rate'
            type='number'
            min={0.01}
            max={100}
            step={0.01}
            value={rate}
            onChange={(event) => setRate(event.target.value)}
            aria-invalid={rate !== '' && Boolean(rateError)}
          />
          {rate !== '' && rateError && (
            <p className='text-destructive text-xs'>{rateError}</p>
          )}
        </div>
      </Dialog>
    </div>
  )
}
