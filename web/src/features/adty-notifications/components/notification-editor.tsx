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
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { DraftDiscardDialog } from '@/features/system-settings/components/draft-discard-dialog'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { getUserSites } from '@/features/users/api'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  notificationManagerKey,
  savePublication,
  type NotificationMode,
  type Publication,
} from '../api'
import {
  notificationAudienceLabels,
  notificationKindLabels,
  notificationSchema,
  type NotificationInput,
} from '../lib/schema'

export function NotificationEditor(props: {
  mode: NotificationMode
  siteId: number
  userId: number
  row: Publication | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const client = useQueryClient()
  const [discarding, setDiscarding] = useState(false)
  const form = useForm<NotificationInput>({
    resolver: zodResolver(notificationSchema),
    defaultValues: props.row ?? {
      title: '',
      content: '',
      kind: 'platform',
      audience: props.mode === 'owner' ? 'site' : 'all_users',
      target_site_id: props.siteId,
      business: '',
    },
  })
  const audience = form.watch('audience')
  useDraftNavigation(form.formState.isDirty)
  const sites = useQuery({
    queryKey: ['adty-notification-targets', props.userId],
    queryFn: async () => requireServerSuccess(await getUserSites()).data,
    enabled: props.mode === 'root' && audience === 'site',
  })
  const save = useMutation({
    mutationFn: (input: NotificationInput) => {
      const user = useAuthStore.getState().auth.user
      if (
        user?.id !== props.userId ||
        (props.mode === 'root' && user.role < ROLE.SUPER_ADMIN)
      ) {
        throw new Error(t('Unauthorized, insufficient privileges'))
      }
      return savePublication(props.mode, props.row, {
        ...input,
        target_site_id: input.audience === 'site' ? input.target_site_id : 0,
      })
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: notificationManagerKey })
      void client.invalidateQueries({ queryKey: ['adty-notifications'] })
      props.onClose()
    },
    onError: (error) => handleServerError(error),
  })
  const close = () => {
    if (save.isPending) return
    if (form.formState.isDirty) setDiscarding(true)
    else props.onClose()
  }
  const errors = form.formState.errors
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) close()
        }}
        title={t(props.row ? 'Edit notification' : 'New notification')}
        description={t(
          'Published notifications appear in the recipient notification center.'
        )}
        footer={
          <>
            <Button variant='outline' disabled={save.isPending} onClick={close}>
              {t('Cancel')}
            </Button>
            <Button type='submit' form={id} disabled={save.isPending}>
              {t(props.row ? 'Save changes' : 'Publish notification')}
            </Button>
          </>
        }
      >
        <form
          id={id}
          onSubmit={form.handleSubmit((input) => save.mutate(input))}
        >
          <FieldGroup>
            <Field data-invalid={!!errors.title}>
              <FieldLabel htmlFor={`${id}-title`}>{t('Title')}</FieldLabel>
              <Input
                id={`${id}-title`}
                maxLength={120}
                aria-invalid={!!errors.title}
                disabled={save.isPending}
                {...form.register('title')}
              />
              <FieldError>
                {errors.title?.message ? t(errors.title.message) : null}
              </FieldError>
            </Field>
            <Field data-invalid={!!errors.content}>
              <FieldLabel htmlFor={`${id}-content`}>{t('Content')}</FieldLabel>
              <Textarea
                id={`${id}-content`}
                maxLength={10000}
                rows={6}
                aria-invalid={!!errors.content}
                disabled={save.isPending}
                {...form.register('content')}
              />
              <FieldError>
                {errors.content?.message ? t(errors.content.message) : null}
              </FieldError>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-kind`}>
                {t('Notification type')}
              </FieldLabel>
              <NativeSelect
                id={`${id}-kind`}
                disabled={save.isPending}
                {...form.register('kind')}
              >
                {Object.entries(notificationKindLabels).map(
                  ([value, label]) => (
                    <NativeSelectOption key={value} value={value}>
                      {t(label)}
                    </NativeSelectOption>
                  )
                )}
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-audience`}>
                {t('Recipients')}
              </FieldLabel>
              <NativeSelect
                id={`${id}-audience`}
                disabled={save.isPending || props.mode === 'owner'}
                {...form.register('audience')}
              >
                {Object.entries(notificationAudienceLabels)
                  .filter(
                    ([value]) => props.mode === 'root' || value === 'site'
                  )
                  .map(([value, label]) => (
                    <NativeSelectOption key={value} value={value}>
                      {t(label)}
                    </NativeSelectOption>
                  ))}
              </NativeSelect>
            </Field>
            {props.mode === 'root' && audience === 'site' && (
              <Field data-invalid={!!errors.target_site_id}>
                <FieldLabel htmlFor={`${id}-site`}>{t('Site')}</FieldLabel>
                <NativeSelect
                  id={`${id}-site`}
                  disabled={save.isPending || sites.isLoading || sites.isError}
                  aria-invalid={!!errors.target_site_id}
                  {...form.register('target_site_id', { valueAsNumber: true })}
                >
                  <NativeSelectOption value={0}>
                    {t('Select a site')}
                  </NativeSelectOption>
                  {(Array.isArray(sites.data) ? sites.data : [])
                    .filter((site) => site.site_id > 0)
                    .map((site) => (
                      <NativeSelectOption
                        key={site.site_id}
                        value={site.site_id}
                      >
                        {site.site_name}
                      </NativeSelectOption>
                    ))}
                </NativeSelect>
                <FieldError>
                  {errors.target_site_id?.message
                    ? t(errors.target_site_id.message)
                    : null}
                </FieldError>
                {sites.isError && (
                  <ErrorState
                    className='min-h-24'
                    onRetry={() => void sites.refetch()}
                  />
                )}
              </Field>
            )}
            <Field>
              <FieldLabel htmlFor={`${id}-business`}>
                {t('Related business')}
              </FieldLabel>
              <Input
                id={`${id}-business`}
                maxLength={200}
                disabled={save.isPending}
                {...form.register('business')}
              />
            </Field>
          </FieldGroup>
        </form>
      </Dialog>
      <DraftDiscardDialog
        open={discarding}
        onOpenChange={setDiscarding}
        onDiscard={props.onClose}
      />
    </>
  )
}
