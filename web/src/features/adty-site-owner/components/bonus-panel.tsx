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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { handleServerError } from '@/lib/handle-server-error'

import { saveOwnerSiteMethodBonus, siteOwnerKeys } from '../api'
import type { OwnerSite } from '../types'

type MethodRow = { method: string; enabled: boolean; rate: string }

/** Canonical payment-method keys are stored, the labels are translated. */
const METHOD_LABELS: Record<string, string> = {
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
  usdt: 'USDT',
}

const rowValue = (row: MethodRow) =>
  JSON.stringify({ enabled: row.enabled, rate: row.rate })

/** Payment-method gifts for this site's top-ups; the bonus is paid from the owner's earnings. */
export function BonusPanel(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const saved = props.site.bonus_methods
  const [rows, setRows] = useState<MethodRow[]>(() =>
    saved.map(({ method, enabled, rate }) => ({ method, enabled, rate }))
  )
  useEffect(() => {
    setRows(
      props.site.bonus_methods.map(({ method, enabled, rate }) => ({
        method,
        enabled,
        rate,
      }))
    )
  }, [props.site.bonus_methods])
  const savedByMethod = new Map(saved.map((row) => [row.method, rowValue(row)]))
  useDraftNavigation(
    rows.some((row) => savedByMethod.get(row.method) !== rowValue(row))
  )
  const save = useMutation({
    mutationFn: (row: MethodRow) =>
      saveOwnerSiteMethodBonus(row.method, row.enabled, row.rate.trim()),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: siteOwnerKeys.all }),
    onError: (error) => handleServerError(error),
  })
  const closing = props.site.status === 'closing'
  const update = (method: string, patch: Partial<MethodRow>) =>
    setRows((prev) =>
      prev.map((row) => (row.method === method ? { ...row, ...patch } : row))
    )

  return (
    <div className='flex flex-col gap-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Your customers get this bonus on top-ups with the chosen payment method. It is paid from your earnings, and only methods the platform authorized for this site are shown.'
        )}
      </p>
      {rows.length === 0 && (
        <p className='text-sm'>
          {t(
            'No payment method is authorized for a gift yet. Ask the platform operator to authorize one.'
          )}
        </p>
      )}
      {rows.map((row) => (
        <div
          key={row.method}
          className='flex flex-col gap-3 border-b pb-4 last:border-b-0 last:pb-0'
        >
          <div className='flex items-center justify-between gap-4'>
            <Label htmlFor={`bonus-method-${row.method}`}>
              {t(METHOD_LABELS[row.method] ?? row.method)}
            </Label>
            <Switch
              id={`bonus-method-${row.method}`}
              checked={row.enabled}
              disabled={closing}
              onCheckedChange={(enabled) => update(row.method, { enabled })}
            />
          </div>
          <div className='flex flex-col gap-1'>
            <Label htmlFor={`bonus-rate-${row.method}`}>
              {t('Bonus rate')}
            </Label>
            <Input
              id={`bonus-rate-${row.method}`}
              inputMode='decimal'
              autoComplete='off'
              value={row.rate}
              disabled={closing}
              onChange={(event) =>
                update(row.method, { rate: event.target.value })
              }
            />
            <span className='text-muted-foreground text-sm'>
              {t('A rate of 0.025 pays 2.5% of the top-up.')}
            </span>
          </div>
          <Button
            size='sm'
            className='self-start'
            disabled={
              closing ||
              save.isPending ||
              savedByMethod.get(row.method) === rowValue(row)
            }
            onClick={() => save.mutate(row)}
          >
            {t('Save')}
          </Button>
        </div>
      ))}
    </div>
  )
}
