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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { PromoCode, PromoCodeInput } from '@/features/adty-marketing/api'
import {
  dateInputToUnix,
  unixToDateInput,
} from '@/features/adty-marketing/lib/campaign'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestamp } from '@/lib/format'

import {
  deleteOwnerPromoCode,
  getOwnerPromoCodes,
  OWNER_PAGE_SIZE,
  saveOwnerPromoCode,
  siteOwnerKeys,
} from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'

// Owner codes are always a percent of the face value, for this site only, funded by the owner.
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

/** The owner's own promo codes; the extra bonus is paid from the owner's earnings (task 12.4). */
export function PromoCodesPanel() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<{
    id: number | null
    input: PromoCodeInput
  } | null>(null)
  const [deleting, setDeleting] = useState<PromoCode | null>(null)
  const query = useQuery({
    queryKey: siteOwnerKeys.list('promo-codes', { p: page }),
    queryFn: () => getOwnerPromoCodes(page),
  })
  const save = useVerifiedOwnerMutation<{
    id: number | null
    input: PromoCodeInput
  }>({
    action: 'promo',
    target: (vars) => vars.id ?? 0,
    run: (vars, proof) => saveOwnerPromoCode(vars.id, vars.input, proof),
    onSuccess: () => setEditing(null),
  })
  const remove = useVerifiedOwnerMutation<number>({
    action: 'promo',
    target: (id) => id,
    run: (id, proof) => deleteOwnerPromoCode(id, proof),
    successMessage: t('Deleted successfully'),
    onSuccess: () => setDeleting(null),
  })
  const input = editing?.input
  const set = (patch: Partial<PromoCodeInput>) =>
    setEditing((prev) =>
      prev ? { ...prev, input: { ...prev.input, ...patch } } : prev
    )

  return (
    <div className='flex flex-col gap-3'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Promo codes add bonus credit to a paid top-up on your site. The bonus is taken from your earnings.'
        )}
      </p>
      <div>
        <Button
          size='sm'
          onClick={() => setEditing({ id: null, input: EMPTY })}
        >
          {t('New promo code')}
        </Button>
      </div>
      <StaticDataTable
        tableClassName='min-w-[640px]'
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
            cell: (row) => `${Number(row.value)}%`,
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
            cell: (row) => (
              <div className='flex gap-2'>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    setEditing({ id: row.id, input: toInput(row) })
                  }
                >
                  {t('Edit')}
                </Button>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() => setDeleting(row)}
                >
                  {t('Delete')}
                </Button>
              </div>
            ),
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={OWNER_PAGE_SIZE}
        onPageChange={setPage}
      />
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => !open && setEditing(null)}
        title={editing?.id ? t('Edit promo code') : t('New promo code')}
        footer={
          <Button
            disabled={
              save.isPending ||
              !input?.value ||
              (editing?.id == null && (input?.code.trim().length ?? 0) < 8)
            }
            onClick={() => editing && save.mutate(editing)}
          >
            {t('Save')}
          </Button>
        }
      >
        {input && (
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='owner-promo-code'>{t('Code')}</Label>
              <Input
                id='owner-promo-code'
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
              <Label htmlFor='owner-promo-value'>{t('Bonus %')}</Label>
              <Input
                id='owner-promo-value'
                type='number'
                min={0}
                step='0.1'
                value={input.value}
                onChange={(event) => set({ value: event.target.value })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='owner-promo-min'>
                {t('Minimum top-up (USD)')}
              </Label>
              <Input
                id='owner-promo-min'
                type='number'
                min={0}
                step={1}
                value={input.min_face_usd}
                onChange={(event) => set({ min_face_usd: event.target.value })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='owner-promo-max'>
                {t('Total uses (0 = unlimited)')}
              </Label>
              <Input
                id='owner-promo-max'
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
              <Label htmlFor='owner-promo-per-user'>
                {t('Uses per user (0 = unlimited)')}
              </Label>
              <Input
                id='owner-promo-per-user'
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
              <Label htmlFor='owner-promo-expires'>
                {t('Valid until (exclusive)')}
              </Label>
              <Input
                id='owner-promo-expires'
                type='date'
                value={unixToDateInput(input.expires_at)}
                onChange={(event) =>
                  set({ expires_at: dateInputToUnix(event.target.value) })
                }
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
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('Delete promo code')}
        desc={t('Orders already placed with this code keep their bonus.')}
        destructive
        isLoading={remove.isPending}
        handleConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </div>
  )
}
