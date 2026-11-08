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

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { formatTimestamp } from '@/lib/format'

import { savePricing } from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'
import { newRowKey, pendingIncreases, siteMarkups } from '../lib/site-owner'
import type { OwnerSite } from '../types'

type GroupRow = { key: string; group: string; markup: string }

/** Site-wide markup and per-group markups, with pending increases (task 11.4). */
export function PricingPanel(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const [defaultMarkup, setDefaultMarkup] = useState('')
  const [rows, setRows] = useState<GroupRow[]>([])
  const [removed, setRemoved] = useState<string[]>([])
  useEffect(() => {
    const current = siteMarkups(props.site.prices)
    setDefaultMarkup(current.defaultMarkup)
    setRows(
      Object.entries(current.groups).map(([group, markup]) => ({
        key: newRowKey(),
        group,
        markup,
      }))
    )
    setRemoved([])
  }, [props.site.prices])
  const savedMarkups = siteMarkups(props.site.prices)
  useDraftNavigation(
    defaultMarkup !== savedMarkups.defaultMarkup ||
      removed.length > 0 ||
      JSON.stringify(
        Object.fromEntries(rows.map((row) => [row.group, row.markup]))
      ) !== JSON.stringify(savedMarkups.groups)
  )
  const pending = pendingIncreases(props.site.prices)
  const save = useVerifiedOwnerMutation<void>({
    action: 'pricing',
    target: () => 0,
    run: (_vars, proof) => {
      const groups: Record<string, string> = {}
      for (const row of rows) {
        if (row.group.trim() && row.markup) {
          groups[row.group.trim()] = row.markup
        }
      }
      // "0" asks the server to drop a group's own price.
      for (const group of removed) groups[group] = '0'
      return savePricing(
        {
          default_markup: defaultMarkup || undefined,
          group_markups: groups,
          expected_version: props.site.pricing_version,
        },
        proof
      )
    },
  })
  const paused = props.site.status !== 'active'

  return (
    <div className='flex flex-col gap-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Customers pay the main site price times your markup. Lowering a markup applies at once; raising it applies after {{hours}} hours, and customers are told by email.',
          { hours: props.site.markup_increase_delay_hours }
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Allowed markup: {{min}} to {{max}}. Your cost factor is {{cost}}.',
          {
            min: props.site.markup_min,
            max: props.site.markup_max || '—',
            cost: props.site.cost_factor,
          }
        )}
      </p>
      {paused && (
        <Alert>
          <AlertDescription>
            {t('Prices cannot be changed while the site is paused')}
          </AlertDescription>
        </Alert>
      )}
      {pending.length > 0 && (
        <section aria-label={t('Pending increases')} className='space-y-1'>
          <h3 className='text-sm font-medium'>{t('Pending increases')}</h3>
          <ul className='text-sm'>
            {pending.map((item) => (
              <li key={item.group || 'default'} data-testid='pending-increase'>
                {t('{{group}}: {{current}} → {{next}} from {{time}}', {
                  group: item.group || t('All groups'),
                  current: item.current,
                  next: item.next,
                  time: formatTimestamp(item.at),
                })}
              </li>
            ))}
          </ul>
        </section>
      )}
      <div className='max-w-xs space-y-2'>
        <Label htmlFor='default-markup'>{t('Default markup')}</Label>
        <Input
          id='default-markup'
          type='number'
          min={1}
          step='0.01'
          value={defaultMarkup}
          disabled={paused}
          onChange={(event) => setDefaultMarkup(event.target.value)}
        />
      </div>
      <section aria-label={t('Markup by group')} className='space-y-2'>
        <h3 className='text-sm font-medium'>{t('Markup by group')}</h3>
        {rows.map((row, index) => (
          <div key={row.key} className='flex items-end gap-2'>
            <div className='space-y-2'>
              <Label htmlFor={`group-name-${index}`}>{t('Group')}</Label>
              <Input
                id={`group-name-${index}`}
                value={row.group}
                disabled={paused}
                onChange={(event) =>
                  setRows((prev) =>
                    prev.map((item, i) =>
                      i === index
                        ? { ...item, group: event.target.value }
                        : item
                    )
                  )
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`group-markup-${index}`}>{t('Markup')}</Label>
              <Input
                id={`group-markup-${index}`}
                type='number'
                min={1}
                step='0.01'
                value={row.markup}
                disabled={paused}
                onChange={(event) =>
                  setRows((prev) =>
                    prev.map((item, i) =>
                      i === index
                        ? { ...item, markup: event.target.value }
                        : item
                    )
                  )
                }
              />
            </div>
            <Button
              variant='outline'
              disabled={paused}
              onClick={() => {
                if (row.group.trim()) {
                  setRemoved((prev) => [...prev, row.group.trim()])
                }
                setRows((prev) => prev.filter((_, i) => i !== index))
              }}
            >
              {t('Remove')}
            </Button>
          </div>
        ))}
        <Button
          size='sm'
          variant='outline'
          disabled={paused}
          onClick={() =>
            setRows((prev) => [
              ...prev,
              { key: newRowKey(), group: '', markup: '' },
            ])
          }
        >
          {t('Add group')}
        </Button>
      </section>
      <div>
        <Button
          disabled={paused || save.isPending}
          onClick={() => save.mutate()}
        >
          {t('Save prices')}
        </Button>
      </div>
    </div>
  )
}
