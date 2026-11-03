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
import { RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { api } from '@/lib/api'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { SettingsSection } from '../components/settings-section'
import {
  OfficialPriceSheet,
  type OfficialPriceInput,
  type OfficialPriceRow,
} from './adty-official-price-sheet'

type ApiResponse<T> = { success: boolean; message?: string; data: T }

const QUERY_KEY = ['adty-admin-official-prices'] as const
const UNMAPPED = '__unmapped'
const NO_ROWS: OfficialPriceRow[] = []

const PRICE_COLUMNS = [
  { field: 'input_per_mtok', labelKey: 'Input' },
  { field: 'output_per_mtok', labelKey: 'Output' },
  { field: 'cache_read_per_mtok', labelKey: 'Cache Read' },
  { field: 'cache_write_per_mtok', labelKey: 'Cache Write' },
  { field: 'per_call', labelKey: 'Per-request' },
] as const

function providerOf(row: OfficialPriceRow): string {
  return row.source_id.split('/')[0] ?? ''
}

/** Official prices (adty task 8.5): models.dev mapping, manual prices and sync. Root only. */
export function AdtyOfficialPricesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [provider, setProvider] = useState('')
  const [keyword, setKeyword] = useState('')
  const [editing, setEditing] = useState<OfficialPriceRow | null>(null)

  const query = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await api.get<ApiResponse<OfficialPriceRow[]>>(
        '/api/adty/admin/official-prices'
      )
      return requireServerSuccess(res.data).data ?? []
    },
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: QUERY_KEY })

  const sync = useMutation({
    mutationFn: async () => {
      const res = await api.post<ApiResponse<{ updated: number }>>(
        '/api/adty/admin/official-prices/sync'
      )
      return requireServerSuccess(res.data).data
    },
    onSuccess: (data) => {
      toast.success(
        t('Official prices synced, {{updated}} updated', {
          updated: data?.updated ?? 0,
        })
      )
      void refresh()
    },
    onError: (error) => handleServerError(error),
  })

  const save = useMutation({
    mutationFn: async (args: { model: string; input: OfficialPriceInput }) => {
      // Model names may contain slashes; the route is /*model, so keep them.
      const path = args.model.split('/').map(encodeURIComponent).join('/')
      const res = await api.put<ApiResponse<OfficialPriceRow>>(
        `/api/adty/admin/official-prices/${path}`,
        args.input
      )
      requireServerSuccess(res.data)
    },
    onSuccess: () => {
      toast.success(t('Official price saved'))
      setEditing(null)
      void refresh()
    },
    onError: (error) => handleServerError(error),
  })

  const rows = query.data ?? NO_ROWS
  const providers = useMemo(
    () =>
      [...new Set(rows.map(providerOf).filter(Boolean))].sort((a, b) =>
        a.localeCompare(b)
      ),
    [rows]
  )
  const needle = keyword.trim().toLowerCase()
  const visible = rows.filter((row) => {
    if (provider === UNMAPPED && row.source_id !== '') return false
    if (provider && provider !== UNMAPPED && providerOf(row) !== provider) {
      return false
    }
    if (!needle) return true
    return (
      row.model_name.toLowerCase().includes(needle) ||
      row.source_id.toLowerCase().includes(needle)
    )
  })

  return (
    <SettingsSection title={t('Official Prices')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Official prices in USD per 1M tokens, used for the savings column on the pricing page. Mapped rows sync from models.dev every day unless manual override is on.'
        )}
      </p>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-2'>
          <Label htmlFor='adty-official-provider'>{t('Provider')}</Label>
          <NativeSelect
            id='adty-official-provider'
            value={provider}
            onChange={(event) => setProvider(event.target.value)}
          >
            <option value=''>{t('All providers')}</option>
            <option value={UNMAPPED}>{t('Not mapped')}</option>
            {providers.map((item) => (
              <option key={item} value={item}>
                {item}
              </option>
            ))}
          </NativeSelect>
        </div>
        <div className='space-y-2'>
          <Label htmlFor='adty-official-search'>{t('Search model ID')}</Label>
          <Input
            id='adty-official-search'
            className='w-64'
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
          />
        </div>
        <Button
          variant='outline'
          className='ml-auto'
          disabled={sync.isPending}
          onClick={() => sync.mutate()}
        >
          <RefreshCw className='mr-2 h-4 w-4' />
          {t('Sync now')}
        </Button>
      </div>
      <StaticDataTable
        tableClassName='min-w-[1100px]'
        tableProps={{ 'aria-label': t('Official Prices') }}
        data={visible}
        getRowKey={(row) => row.model_name}
        emptyContent={
          <EmptyState
            className='min-h-32'
            title={
              query.isLoading
                ? t('Loading...')
                : t('No models match the filter')
            }
          />
        }
        columns={[
          {
            id: 'model',
            header: t('Model'),
            cellClassName: 'font-mono',
            cell: (row) => row.model_name,
          },
          {
            id: 'source_id',
            header: t('models.dev ID'),
            cellClassName: 'font-mono',
            cell: (row) => row.source_id || '-',
          },
          ...PRICE_COLUMNS.map((column) => ({
            id: column.field,
            header: t(column.labelKey),
            cellClassName: 'font-mono tabular-nums',
            cell: (row: OfficialPriceRow) => row[column.field] ?? '-',
          })),
          {
            id: 'override',
            header: t('Manual override'),
            cell: (row) =>
              row.override ? (
                <StatusBadge
                  label={t('On')}
                  variant='warning'
                  copyable={false}
                />
              ) : (
                '-'
              ),
          },
          {
            id: 'source',
            header: t('Source'),
            cell: (row) => row.source || '-',
          },
          {
            id: 'synced_at',
            header: t('Synced at'),
            cell: (row) => formatTimestampToDate(row.synced_at),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (row) => (
              <Button size='sm' variant='ghost' onClick={() => setEditing(row)}>
                {t('Edit')}
              </Button>
            ),
          },
        ]}
      />
      <OfficialPriceSheet
        row={editing}
        isSaving={save.isPending}
        onOpenChange={(open) => !open && setEditing(null)}
        onSave={(input) =>
          editing && save.mutate({ model: editing.model_name, input })
        }
      />
    </SettingsSection>
  )
}
