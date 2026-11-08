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
import dayjs from 'dayjs'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  deleteInviteCode,
  getInviteCodes,
  saveInviteCode,
  SITE_ADMIN_PAGE_SIZE,
  siteAdminKeys,
} from '../api'
import type { InviteCode } from '../types'

type Draft = {
  code: string
  max_uses: string
  expire_date: string
  cost_factor: string
  max_markup: string
  note: string
}

const EMPTY_DRAFT: Draft = {
  code: '',
  max_uses: '1',
  expire_date: '',
  cost_factor: '',
  max_markup: '',
  note: '',
}

function toDraft(code: InviteCode): Draft {
  return {
    code: code.code,
    max_uses: String(code.max_uses),
    expire_date:
      code.expire_at > 0 ? dayjs.unix(code.expire_at).format('YYYY-MM-DD') : '',
    cost_factor: code.cost_factor ?? '',
    max_markup: code.max_markup ?? '',
    note: code.note,
  }
}

/** Site opening invite codes (task 11.1): a valid code opens the site with its preset d and k. */
export function InviteCodesPanel(props: {
  canCreate: boolean
  canWrite: boolean
  canDelete: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<InviteCode | 'new' | null>(null)
  const [deleting, setDeleting] = useState<InviteCode | null>(null)
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT)
  const canSave =
    editing === 'new' ? props.canCreate : editing !== null && props.canWrite
  const query = useQuery({
    queryKey: siteAdminKeys.list('invite-codes', { page }),
    queryFn: () => getInviteCodes(page),
  })
  const done = () => {
    toast.success(t('Saved successfully'))
    setEditing(null)
    setDeleting(null)
    void queryClient.invalidateQueries({ queryKey: siteAdminKeys.all })
  }
  const save = useMutation({
    mutationFn: () => {
      const action =
        editing === 'new'
          ? ADTY_PERMISSIONS.SITE_SETTINGS_CREATE
          : ADTY_PERMISSIONS.SITE_SETTINGS
      if (!canSave || !canAdmin(useAuthStore.getState().auth.user, action)) {
        throw new Error(t('No permission to perform this action'))
      }
      return saveInviteCode(
        editing === 'new' || editing === null ? null : editing.id,
        {
          code: editing === 'new' ? draft.code.trim() || undefined : undefined,
          max_uses: Number(draft.max_uses) || 0,
          expire_at: draft.expire_date
            ? dayjs(draft.expire_date).endOf('day').unix()
            : 0,
          cost_factor: draft.cost_factor.trim() || undefined,
          max_markup: draft.max_markup.trim() || undefined,
          note: draft.note.trim(),
        }
      )
    },
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const remove = useMutation({
    mutationFn: (id: number) => {
      if (
        !props.canDelete ||
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.SITE_SETTINGS_DELETE
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return deleteInviteCode(id)
    },
    onSuccess: done,
    onError: (err) => handleServerError(err),
  })
  const set = (key: keyof Draft, value: string) =>
    setDraft((prev) => ({ ...prev, [key]: value }))

  return (
    <div className='flex flex-col gap-3'>
      {props.canCreate && (
        <div>
          <Button
            onClick={() => {
              setDraft(EMPTY_DRAFT)
              setEditing('new')
            }}
          >
            {t('New invite code')}
          </Button>
        </div>
      )}
      <StaticDataTable
        tableClassName='min-w-[760px]'
        tableProps={{ 'aria-label': t('Invite codes') }}
        data={query.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={<EmptyState className='min-h-32' title={t('No data')} />}
        columns={[
          {
            id: 'code',
            header: t('Code'),
            cell: (row) => <span className='font-mono'>{row.code}</span>,
          },
          {
            id: 'uses',
            header: t('Used'),
            cell: (row) =>
              row.max_uses > 0
                ? `${row.used_count} / ${row.max_uses}`
                : `${row.used_count} / ${t('Unlimited')}`,
          },
          {
            id: 'expire',
            header: t('Expires'),
            cell: (row) =>
              row.expire_at > 0
                ? formatTimestampToDate(row.expire_at)
                : t('Never'),
          },
          {
            id: 'preset',
            header: t('Preset d / k'),
            cell: (row) =>
              `${row.cost_factor ?? t('Global')} / ${row.max_markup ?? t('Global')}`,
          },
          { id: 'note', header: t('Note'), cell: (row) => row.note || '-' },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (row) =>
              props.canWrite || props.canDelete ? (
                <div className='flex justify-end gap-1'>
                  {props.canWrite && (
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => {
                        setDraft(toDraft(row))
                        setEditing(row)
                      }}
                    >
                      {t('Edit')}
                    </Button>
                  )}
                  {props.canDelete && (
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setDeleting(row)}
                    >
                      {t('Delete')}
                    </Button>
                  )}
                </div>
              ) : null,
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={query.data?.total ?? 0}
        pageSize={SITE_ADMIN_PAGE_SIZE}
        onPageChange={setPage}
      />
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => !open && setEditing(null)}
        title={editing === 'new' ? t('New invite code') : t('Edit invite code')}
        footer={
          <>
            <Button variant='outline' onClick={() => setEditing(null)}>
              {t('Cancel')}
            </Button>
            <Button
              disabled={!canSave || save.isPending}
              onClick={() => canSave && save.mutate()}
            >
              {t('Save')}
            </Button>
          </>
        }
      >
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='space-y-1'>
            <Label htmlFor='invite-code'>{t('Code')}</Label>
            <Input
              id='invite-code'
              value={draft.code}
              disabled={!canSave || editing !== 'new'}
              placeholder={t('Leave empty to generate')}
              onChange={(event) => set('code', event.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label htmlFor='invite-max-uses'>
              {t('Max uses (0 for unlimited)')}
            </Label>
            <Input
              id='invite-max-uses'
              disabled={!canSave}
              type='number'
              min={0}
              value={draft.max_uses}
              onChange={(event) => set('max_uses', event.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label htmlFor='invite-expire'>
              {t('Expires on (empty for never)')}
            </Label>
            <Input
              id='invite-expire'
              disabled={!canSave}
              type='date'
              value={draft.expire_date}
              onChange={(event) => set('expire_date', event.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label htmlFor='invite-cost-factor'>
              {t('Preset cost factor')}
            </Label>
            <Input
              id='invite-cost-factor'
              disabled={!canSave}
              value={draft.cost_factor}
              placeholder={t('Global')}
              onChange={(event) => set('cost_factor', event.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label htmlFor='invite-max-markup'>
              {t('Preset markup limit')}
            </Label>
            <Input
              id='invite-max-markup'
              disabled={!canSave}
              value={draft.max_markup}
              placeholder={t('Global')}
              onChange={(event) => set('max_markup', event.target.value)}
            />
          </div>
          <div className='space-y-1'>
            <Label htmlFor='invite-note'>{t('Note')}</Label>
            <Input
              id='invite-note'
              disabled={!canSave}
              value={draft.note}
              onChange={(event) => set('note', event.target.value)}
            />
          </div>
        </div>
      </Dialog>
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('Delete invite code')}
        desc={t('Sites already opened with this code are not affected.')}
        destructive
        confirmText={t('Delete')}
        isLoading={remove.isPending}
        disabled={!props.canDelete}
        handleConfirm={() =>
          props.canDelete && deleting && remove.mutate(deleting.id)
        }
      />
    </div>
  )
}
