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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { saveBonusRates } from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'
import { newRowKey } from '../lib/site-owner'
import type { OwnerSite } from '../types'

type TierRow = { key: string; tier: string; rate: string }

/** Bonus tiers for this site's top-ups; the bonus is paid from the owner's earnings (task 11.5). */
export function BonusPanel(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<TierRow[]>([])
  useEffect(() => {
    setRows(
      Object.entries(props.site.bonus_rates ?? {})
        .map(([tier, rate]) => ({ key: newRowKey(), tier, rate: String(rate) }))
        .sort((a, b) => Number(a.tier) - Number(b.tier))
    )
  }, [props.site.bonus_rates])
  const save = useVerifiedOwnerMutation<void>({
    action: 'bonus',
    target: () => props.site.id,
    run: (_vars, proof) => {
      const rates: Record<string, number> = {}
      for (const row of rows) {
        if (row.tier && row.rate) {
          rates[String(Math.floor(Number(row.tier)))] = Number(row.rate)
        }
      }
      return saveBonusRates(rates, proof)
    },
  })
  const paused = props.site.status !== 'active'
  const update = (index: number, patch: Partial<TierRow>) =>
    setRows((prev) =>
      prev.map((row, i) => (i === index ? { ...row, ...patch } : row))
    )

  return (
    <div className='flex flex-col gap-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Your customers get this bonus on top-ups of at least the tier amount. The bonus is taken from your earnings, and each tier may not exceed the main site bonus by more than the platform allows.'
        )}
      </p>
      {rows.map((row, index) => (
        <div key={row.key} className='flex items-end gap-2'>
          <div className='space-y-2'>
            <Label htmlFor={`bonus-tier-${index}`}>
              {t('Top-up from (USD)')}
            </Label>
            <Input
              id={`bonus-tier-${index}`}
              type='number'
              min={1}
              step={1}
              value={row.tier}
              disabled={paused}
              onChange={(event) => update(index, { tier: event.target.value })}
            />
          </div>
          <div className='space-y-2'>
            <Label htmlFor={`bonus-rate-${index}`}>{t('Bonus %')}</Label>
            <Input
              id={`bonus-rate-${index}`}
              type='number'
              min={0}
              step='0.1'
              value={row.rate}
              disabled={paused}
              onChange={(event) => update(index, { rate: event.target.value })}
            />
          </div>
          <Button
            variant='outline'
            disabled={paused}
            onClick={() =>
              setRows((prev) => prev.filter((_, i) => i !== index))
            }
          >
            {t('Remove')}
          </Button>
        </div>
      ))}
      <div className='flex gap-2'>
        <Button
          size='sm'
          variant='outline'
          disabled={paused}
          onClick={() =>
            setRows((prev) => [
              ...prev,
              { key: newRowKey(), tier: '', rate: '' },
            ])
          }
        >
          {t('Add tier')}
        </Button>
        <Button
          size='sm'
          disabled={paused || save.isPending}
          onClick={() => save.mutate()}
        >
          {t('Save bonuses')}
        </Button>
      </div>
    </div>
  )
}
