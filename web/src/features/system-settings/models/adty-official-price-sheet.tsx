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

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
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
import { Switch } from '@/components/ui/switch'

type PriceField =
  | 'input_per_mtok'
  | 'output_per_mtok'
  | 'cache_read_per_mtok'
  | 'cache_write_per_mtok'
  | 'per_call'

/** One row of GET /api/adty/admin/official-prices; decimals arrive as strings. */
export type OfficialPriceRow = Record<PriceField, string | null> & {
  model_name: string
  source_id: string
  source: string
  override: boolean
  synced_at: number
  updated_at: number
}

/** Body of PUT /api/adty/admin/official-prices/<model>; the row is replaced. */
export type OfficialPriceInput = Record<PriceField, string | null> & {
  source_id: string
  override: boolean
}

const PRICE_FIELDS: Array<{ field: PriceField; labelKey: string }> = [
  { field: 'input_per_mtok', labelKey: 'Input' },
  { field: 'output_per_mtok', labelKey: 'Output' },
  { field: 'cache_read_per_mtok', labelKey: 'Cache Read' },
  { field: 'cache_write_per_mtok', labelKey: 'Cache Write' },
  { field: 'per_call', labelKey: 'Per-request' },
]

type Draft = Record<PriceField, string> & {
  source_id: string
  override: boolean
}

function toDraft(row: OfficialPriceRow | null): Draft {
  const draft = {
    source_id: row?.source_id ?? '',
    override: row?.override ?? false,
  } as Draft
  for (const item of PRICE_FIELDS) {
    draft[item.field] = row?.[item.field] ?? ''
  }
  return draft
}

const SOURCE_ID_PATTERN = /^[^/\s]+\/\S+$/

/** Edit the models.dev mapping, prices and override switch of one model. */
export function OfficialPriceSheet(props: {
  row: OfficialPriceRow | null
  isSaving: boolean
  onOpenChange: (open: boolean) => void
  onSave: (input: OfficialPriceInput) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<Draft>(() => toDraft(props.row))
  useEffect(() => {
    setDraft(toDraft(props.row))
  }, [props.row])

  const sourceId = draft.source_id.trim()
  const sourceIdInvalid = sourceId !== '' && !SOURCE_ID_PATTERN.test(sourceId)
  const priceErrors = new Set<PriceField>()
  for (const item of PRICE_FIELDS) {
    const value = draft[item.field].trim()
    if (value !== '' && !(Number(value) >= 0)) priceErrors.add(item.field)
  }

  const submit = () => {
    if (sourceIdInvalid || priceErrors.size > 0) return
    const input = {
      source_id: sourceId,
      override: draft.override,
    } as OfficialPriceInput
    for (const item of PRICE_FIELDS) {
      const value = draft[item.field].trim()
      input[item.field] = value === '' ? null : value
    }
    props.onSave(input)
  }

  return (
    <Sheet open={props.row !== null} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName()}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle className='font-mono'>{props.row?.model_name}</SheetTitle>
          <SheetDescription>
            {t(
              'Prices in USD per 1M tokens; per-request price in USD per call. Leave a price empty if the model has none.'
            )}
          </SheetDescription>
        </SheetHeader>
        <form
          className={sideDrawerFormClassName()}
          onSubmit={(event) => {
            event.preventDefault()
            submit()
          }}
        >
          <div className='space-y-2'>
            <Label htmlFor='adty-official-source-id'>
              {t('models.dev ID')}
            </Label>
            <Input
              id='adty-official-source-id'
              className='font-mono'
              placeholder='anthropic/claude-sonnet-4-5'
              value={draft.source_id}
              aria-invalid={sourceIdInvalid}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, source_id: event.target.value }))
              }
            />
            {sourceIdInvalid && (
              <p className='text-destructive text-xs'>
                {t('The models.dev ID must look like provider/model')}
              </p>
            )}
          </div>
          <div className='grid grid-cols-2 gap-3'>
            {PRICE_FIELDS.map((item) => (
              <div key={item.field} className='space-y-2'>
                <Label htmlFor={`adty-official-${item.field}`}>
                  {t(item.labelKey)}
                </Label>
                <Input
                  id={`adty-official-${item.field}`}
                  inputMode='decimal'
                  className='font-mono'
                  value={draft[item.field]}
                  aria-invalid={priceErrors.has(item.field)}
                  onChange={(event) =>
                    setDraft((prev) => ({
                      ...prev,
                      [item.field]: event.target.value,
                    }))
                  }
                />
                {priceErrors.has(item.field) && (
                  <p className='text-destructive text-xs'>
                    {t('Enter a number of at least 0')}
                  </p>
                )}
              </div>
            ))}
          </div>
          <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
            <div className='space-y-1'>
              <Label htmlFor='adty-official-override'>
                {t('Manual override')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t('When on, the daily sync keeps these prices.')}
              </p>
            </div>
            <Switch
              id='adty-official-override'
              checked={draft.override}
              onCheckedChange={(checked) =>
                setDraft((prev) => ({ ...prev, override: checked }))
              }
            />
          </div>
          <div className='flex justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => props.onOpenChange(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              disabled={
                sourceIdInvalid || priceErrors.size > 0 || props.isSaving
              }
            >
              {t('Save')}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  )
}
