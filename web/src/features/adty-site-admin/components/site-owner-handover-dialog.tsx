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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DraftDiscardDialog } from '@/features/system-settings/components/draft-discard-dialog'
import { searchUsers } from '@/features/users/api'
import { ROLE } from '@/lib/roles'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { handoverSiteOwner, siteAdminKeys } from '../api'
import {
  ownerHandoverSchema,
  type OwnerHandoverDraft,
} from '../lib/owner-handover'
import type { AdminSite } from '../types'

export function SiteOwnerHandoverDialog(props: {
  site: AdminSite
  snapshot: AdminSite
  onClose: () => void
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const client = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const [actorId] = useState(user?.id)
  const authorized = user?.id === actorId && user?.role === ROLE.SUPER_ADMIN
  const [keyword, setKeyword] = useState('')
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<{
    value: string
    label: string
  } | null>(null)
  const [discarding, setDiscarding] = useState(false)
  const [pending, setPending] = useState<OwnerHandoverDraft | null>(null)
  const form = useForm<OwnerHandoverDraft>({
    resolver: zodResolver(ownerHandoverSchema),
    defaultValues: { new_owner_user_id: 0, reason: '' },
  })
  const dirty = form.formState.isDirty || keyword !== ''
  const { onDirtyChange } = props
  useEffect(() => {
    onDirtyChange(dirty)
    return () => onDirtyChange(false)
  }, [dirty, onDirtyChange])
  const stale =
    props.site.owner_user_id !== props.snapshot.owner_user_id ||
    props.site.ownership_version !== props.snapshot.ownership_version ||
    !['active', 'paused'].includes(props.site.status)
  const users = useQuery({
    queryKey: [...siteAdminKeys.all, 'owner-search', actorId, search],
    queryFn: async () =>
      requireServerSuccess(
        await searchUsers({
          keyword: search,
          site_id: 0,
          status: '1',
          p: 1,
          page_size: 20,
        })
      ).data,
    enabled: authorized && search !== '',
  })
  const options = (users.data?.items ?? [])
    .filter(
      (candidate) =>
        candidate.id !== props.snapshot.owner_user_id &&
        candidate.status === 1 &&
        candidate.site_id === 0
    )
    .map((candidate) => ({
      value: String(candidate.id),
      label: `${candidate.username} (#${candidate.id})`,
    }))
  if (selected && !options.some((option) => option.value === selected.value)) {
    options.push(selected)
  }
  const save = useMutation({
    mutationFn: (draft: OwnerHandoverDraft) => {
      const actor = useAuthStore.getState().auth.user
      if (actor?.id !== actorId || actor?.role !== ROLE.SUPER_ADMIN) {
        throw new Error(t('No permission to perform this action'))
      }
      if (stale) {
        throw new Error(
          t('Site ownership changed; reload the site before trying again')
        )
      }
      return handoverSiteOwner(props.snapshot.id, {
        ...draft,
        expected_owner_user_id: props.snapshot.owner_user_id,
        expected_ownership_version: props.snapshot.ownership_version ?? 0,
      })
    },
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
      props.onClose()
    },
    onError: (error) => {
      setPending(null)
      form.setError('root', {
        message: getServerErrorMessage(error, t('Failed to save')),
      })
    },
  })
  if (!authorized) return null
  const close = () => {
    if (save.isPending) return
    if (dirty) setDiscarding(true)
    else props.onClose()
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) close()
        }}
        title={t('Change site owner')}
        description={t(
          'Historical earnings and refunds stay with their original owner. The new owner must accept the site agreement, and staff need new authorization.'
        )}
        footer={
          <>
            <Button variant='outline' disabled={save.isPending} onClick={close}>
              {t('Cancel')}
            </Button>
            <Button
              form={id}
              type='submit'
              disabled={
                stale ||
                save.isPending ||
                !selected ||
                !form.watch('reason').trim()
              }
            >
              {t('Review handover')}
            </Button>
          </>
        }
      >
        <form id={id} onSubmit={form.handleSubmit(setPending)}>
          <FieldGroup>
            <p className='text-sm break-all'>
              {t('Owner')}:{' '}
              {props.snapshot.owner_username ||
                `#${props.snapshot.owner_user_id}`}
            </p>
            <Field>
              <FieldLabel htmlFor={`${id}-search`}>
                {t('Find a main-site user')}
              </FieldLabel>
              <div className='flex gap-2'>
                <Input
                  id={`${id}-search`}
                  value={keyword}
                  disabled={save.isPending}
                  onChange={(event) => setKeyword(event.target.value)}
                />
                <Button
                  type='button'
                  variant='outline'
                  disabled={
                    !keyword.trim() || users.isFetching || save.isPending
                  }
                  onClick={() => {
                    if (search === keyword.trim()) void users.refetch()
                    else setSearch(keyword.trim())
                  }}
                >
                  {t('Search users')}
                </Button>
              </div>
              {users.isError && (
                <FieldError>{getServerErrorMessage(users.error)}</FieldError>
              )}
              {users.isSuccess && options.length === 0 && (
                <p className='text-muted-foreground text-sm'>{t('No data')}</p>
              )}
            </Field>
            <Controller
              control={form.control}
              name='new_owner_user_id'
              render={({ field, fieldState }) => (
                <Field data-invalid={fieldState.invalid}>
                  <FieldLabel htmlFor={`${id}-owner`}>
                    {t('New site owner')}
                  </FieldLabel>
                  <Combobox
                    id={`${id}-owner`}
                    className='w-full'
                    aria-label={t('New site owner')}
                    options={options}
                    value={field.value > 0 ? String(field.value) : null}
                    disabled={save.isPending}
                    onValueChange={(value) => {
                      field.onChange(Number(value))
                      setSelected(
                        options.find((option) => option.value === value) ?? null
                      )
                    }}
                  />
                  <FieldError errors={[fieldState.error]} />
                </Field>
              )}
            />
            <Field data-invalid={!!form.formState.errors.reason}>
              <FieldLabel htmlFor={`${id}-reason`}>
                {t('Handover reason')}
              </FieldLabel>
              <Textarea
                id={`${id}-reason`}
                {...form.register('reason')}
                maxLength={500}
                disabled={save.isPending}
                aria-invalid={!!form.formState.errors.reason}
              />
              <FieldError errors={[form.formState.errors.reason]} />
            </Field>
            {stale && (
              <p role='alert' className='text-destructive text-sm'>
                {t(
                  'Site ownership changed; reload the site before trying again'
                )}
              </p>
            )}
            <FieldError>{form.formState.errors.root?.message}</FieldError>
          </FieldGroup>
        </form>
      </Dialog>
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setPending(null)
        }}
        title={t('Confirm owner handover')}
        desc={
          <p className='break-all'>
            {props.snapshot.owner_username ||
              `#${props.snapshot.owner_user_id}`}{' '}
            → {selected?.label}
          </p>
        }
        confirmText={t('Change site owner')}
        disabled={stale}
        isLoading={save.isPending}
        handleConfirm={() => {
          if (pending && !stale) save.mutate(pending)
        }}
      />
      <DraftDiscardDialog
        open={discarding}
        onOpenChange={setDiscarding}
        onDiscard={props.onClose}
      />
    </>
  )
}
