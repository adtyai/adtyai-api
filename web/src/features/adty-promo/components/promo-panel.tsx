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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { DraftDiscardDialog } from '@/features/system-settings/components/draft-discard-dialog'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getPromoCodes,
  promoKeys,
  savePromoCode,
  type PromoCode,
  type PromoCodeInput,
} from '../api'
import { dateInputToUnix, unixToDateInput } from '../lib/dates'

const EMPTY: PromoCodeInput = {
  code: '',
  kind: 'percent',
  value: '',
  min_face_usd: '0',
  max_uses: 0,
  per_user_limit: 1,
  starts_at: 0,
  expires_at: 0,
  new_users_only: false,
  site_id: 0,
  enabled: true,
  note: '',
}

function toInput(code: PromoCode): PromoCodeInput {
  return {
    code: code.code,
    kind: code.kind,
    value: code.value,
    min_face_usd: code.min_face_usd,
    max_uses: code.max_uses,
    per_user_limit: code.per_user_limit,
    starts_at: code.starts_at,
    expires_at: code.expires_at,
    new_users_only: code.new_users_only,
    site_id: code.site_id,
    enabled: code.enabled,
    note: code.note,
  }
}

/** Platform-funded promo codes (adty task 12.4); they add bonus credit to a paid top-up. */
export function PromoPanel(props: { canCreate: boolean; canWrite: boolean }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<{
    id: number | null
    input: PromoCodeInput
    initialInput: PromoCodeInput
  } | null>(null)
  const [discarding, setDiscarding] = useState(false)
  const dirty =
    !!editing &&
    JSON.stringify(editing.input) !== JSON.stringify(editing.initialInput)
  useDraftNavigation(dirty)
  const query = useQuery({
    queryKey: promoKeys.promoCodes(page),
    queryFn: () => getPromoCodes(page),
  })
  const save = useMutation({
    mutationFn: () => {
      if (
        !editing ||
        !(editing.id === null ? props.canCreate : props.canWrite) ||
        (useAuthStore.getState().auth.user?.role ?? 0) < ROLE.SUPER_ADMIN
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return savePromoCode(editing.id, editing.input)
    },
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      setEditing(null)
      void queryClient.invalidateQueries({ queryKey: promoKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const set = (patch: Partial<PromoCodeInput>) =>
    setEditing((prev) =>
      prev ? { ...prev, input: { ...prev.input, ...patch } } : prev
    )
  const input = editing?.input
  const canSave =
    editing !== null && (editing.id === null ? props.canCreate : props.canWrite)

  if (query.isError && !query.data) {
    return <ErrorState onRetry={() => void query.refetch()} />
  }

  return (
    <div className='flex flex-col gap-3'>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <p className='text-muted-foreground text-sm'>{t('Promo codes')}</p>
      {props.canCreate && (
        <div>
          <Button
            size='sm'
            onClick={() =>
              setEditing({ id: null, input: EMPTY, initialInput: EMPTY })
            }
          >
            {t('New promo code')}
          </Button>
        </div>
      )}
      <StaticDataTable
        tableClassName='min-w-[860px]'
        tableProps={{ 'aria-label': t('Promo codes') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState className='min-h-32' title={t('No promo codes yet')} />
        }
        columns={[
          {
            id: 'code',
            header: t('Code'),
            cell: (row) => <span className='font-mono'>{row.code}</span>,
          },
          {
            id: 'value',
            header: t('Bonus'),
            cell: (row) =>
              row.kind === 'percent'
                ? `${Number(row.value)}%`
                : formatUsd(Number(row.value), locale),
          },
          {
            id: 'uses',
            header: t('Used'),
            cell: (row) =>
              row.max_uses > 0
                ? `${formatNumber(row.used_count, locale)} / ${formatNumber(row.max_uses, locale)}`
                : formatNumber(row.used_count, locale),
          },
          {
            id: 'expires',
            header: t('Expires'),
            cell: (row) =>
              row.expires_at > 0 ? formatTimestamp(row.expires_at) : t('Never'),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <StatusBadge
                label={row.enabled ? t('Enabled') : t('Disabled')}
                variant={row.enabled ? 'success' : 'neutral'}
              />
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (row) =>
              props.canWrite && (
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    setEditing({
                      id: row.id,
                      input: toInput(row),
                      initialInput: toInput(row),
                    })
                  }
                >
                  {t('Edit')}
                </Button>
              ),
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={20}
        onPageChange={setPage}
      />
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) {
            if (dirty) setDiscarding(true)
            else setEditing(null)
          }
        }}
        title={editing?.id ? t('Edit promo code') : t('New promo code')}
        footer={
          <Button
            disabled={
              save.isPending ||
              !canSave ||
              !input?.value ||
              (editing?.id == null && (input?.code.trim().length ?? 0) < 8)
            }
            onClick={() => save.mutate()}
          >
            {t('Save')}
          </Button>
        }
      >
        {input && (
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='promo-code'>{t('Code')}</Label>
              <Input
                id='promo-code'
                value={input.code}
                disabled={editing?.id !== null}
                minLength={8}
                maxLength={32}
                onChange={(event) =>
                  set({ code: event.target.value.toUpperCase() })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-kind'>{t('Type')}</Label>
              <NativeSelect
                id='promo-kind'
                value={input.kind}
                onChange={(event) =>
                  set({ kind: event.target.value as PromoCodeInput['kind'] })
                }
              >
                <NativeSelectOption value='percent'>
                  {t('Percent of the amount')}
                </NativeSelectOption>
                <NativeSelectOption value='fixed'>
                  {t('Fixed amount (USD)')}
                </NativeSelectOption>
              </NativeSelect>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-value'>
                {input.kind === 'percent' ? t('Bonus %') : t('Bonus (USD)')}
              </Label>
              <Input
                id='promo-value'
                type='number'
                min={0}
                step='0.01'
                value={input.value}
                onChange={(event) => set({ value: event.target.value })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-min'>{t('Minimum top-up (USD)')}</Label>
              <Input
                id='promo-min'
                type='number'
                min={0}
                step={1}
                value={input.min_face_usd}
                onChange={(event) => set({ min_face_usd: event.target.value })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-max'>
                {t('Total uses (0 = unlimited)')}
              </Label>
              <Input
                id='promo-max'
                type='number'
                min={0}
                step={1}
                value={input.max_uses}
                onChange={(event) =>
                  set({ max_uses: Number(event.target.value) })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-per-user'>
                {t('Uses per user (0 = unlimited)')}
              </Label>
              <Input
                id='promo-per-user'
                type='number'
                min={0}
                step={1}
                value={input.per_user_limit}
                onChange={(event) =>
                  set({ per_user_limit: Number(event.target.value) })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-starts'>{t('Valid from')}</Label>
              <Input
                id='promo-starts'
                type='date'
                value={unixToDateInput(input.starts_at)}
                onChange={(event) =>
                  set({ starts_at: dateInputToUnix(event.target.value) })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-expires'>
                {t('Valid until (exclusive)')}
              </Label>
              <Input
                id='promo-expires'
                type='date'
                value={unixToDateInput(input.expires_at)}
                onChange={(event) =>
                  set({ expires_at: dateInputToUnix(event.target.value) })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-site'>{t('Site ID (0 = main site)')}</Label>
              <Input
                id='promo-site'
                type='number'
                min={0}
                step={1}
                value={input.site_id}
                onChange={(event) =>
                  set({ site_id: Number(event.target.value) })
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='promo-note'>{t('Note')}</Label>
              <Input
                id='promo-note'
                maxLength={255}
                value={input.note}
                onChange={(event) => set({ note: event.target.value })}
              />
            </div>
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={input.new_users_only}
                onCheckedChange={(checked) =>
                  set({ new_users_only: checked === true })
                }
              />
              {t('First top-up only')}
            </label>
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={input.enabled}
                onCheckedChange={(checked) =>
                  set({ enabled: checked === true })
                }
              />
              {t('Enabled')}
            </label>
          </div>
        )}
      </Dialog>
      <DraftDiscardDialog
        open={discarding}
        onOpenChange={setDiscarding}
        onDiscard={() => setEditing(null)}
      />
    </div>
  )
}
