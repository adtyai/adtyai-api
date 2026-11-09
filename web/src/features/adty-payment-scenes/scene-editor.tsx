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

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  getEpayChannels,
  epayChannelsQueryKey,
} from '@/features/adty-payment-channels/api'
import { DraftDiscardDialog } from '@/features/system-settings/components/draft-discard-dialog'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { getUserSites } from '@/features/users/api'
import { ROLE } from '@/lib/roles'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  paymentScenesKey,
  savePaymentScenes,
  type PaymentScene,
  type PaymentSceneConfig,
} from './api'
import {
  paymentSceneSchema,
  sceneAudienceLabels,
  sceneUserLabels,
} from './schema'

export function PaymentSceneEditor(props: {
  config: PaymentSceneConfig
  index: number
  userId: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const client = useQueryClient()
  const [discarding, setDiscarding] = useState(false)
  const [pending, setPending] = useState<PaymentScene | null>(null)
  const form = useForm<PaymentScene>({
    resolver: zodResolver(paymentSceneSchema),
    defaultValues: props.config.scenes[props.index] ?? {
      key: '',
      name: '',
      business: 'wallet_topup',
      enabled: false,
      providers: [],
      methods: [],
      channel_ids: [],
      currencies: [],
      site_scope: 'all',
      site_ids: [],
      user_types: [],
      min_usd: '0',
      max_usd: '0',
    },
  })
  useDraftNavigation(form.formState.isDirty)
  const epaySelected = form.watch('providers').includes('adty_epay')
  const selectedSites = form.watch('site_scope') === 'selected'
  const channels = useQuery({
    queryKey: [...epayChannelsQueryKey, props.userId],
    queryFn: getEpayChannels,
    enabled: epaySelected,
  })
  const sites = useQuery({
    queryKey: ['adty-scene-sites', props.userId],
    queryFn: async () => requireServerSuccess(await getUserSites()).data,
    enabled: selectedSites,
  })
  const save = useMutation({
    mutationFn: (scene: PaymentScene) => {
      const user = useAuthStore.getState().auth.user
      if (user?.id !== props.userId || user.role < ROLE.SUPER_ADMIN) {
        throw new Error(t('Unauthorized, insufficient privileges'))
      }
      const scenes = [...props.config.scenes]
      if (props.index < 0) scenes.push(scene)
      else scenes[props.index] = scene
      return savePaymentScenes({ ...props.config, scenes })
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: paymentScenesKey })
      void client.invalidateQueries({ queryKey: ['adty-topup'] })
      props.onClose()
    },
    onError: (error) => {
      setPending(null)
      form.setError('root', {
        message: getServerErrorMessage(error, t('Failed to save')),
      })
    },
  })
  const close = () => {
    if (save.isPending) return
    if (form.formState.isDirty) setDiscarding(true)
    else props.onClose()
  }
  const providerOptions = props.config.provider_options ?? []
  const methodOptions = props.config.method_options ?? []
  const stringChoices = [
    {
      field: 'providers' as const,
      label: 'Payment providers',
      options: providerOptions.map((item) => ({
        value: item.provider,
        label: item.label,
      })),
    },
    {
      field: 'methods' as const,
      label: 'Payment methods',
      options: methodOptions.map((item) => ({
        value: item.id,
        label: item.label || item.id,
      })),
    },
    {
      field: 'currencies' as const,
      label: 'Currencies',
      options: [
        ...new Set(methodOptions.map((item) => item.currency).filter(Boolean)),
      ].map((value) => ({ value, label: value })),
    },
    {
      field: 'user_types' as const,
      label: 'User types',
      options: Object.entries(sceneUserLabels).map(([value, label]) => ({
        value,
        label: t(label),
      })),
    },
  ]
  const errors = form.formState.errors
  return (
    <>
      <Dialog
        open
        title={t(props.index < 0 ? 'New payment scene' : 'Edit payment scene')}
        onOpenChange={(open) => !open && close()}
        footer={
          <Button type='submit' form={`${id}-form`} disabled={save.isPending}>
            {t('Save payment scene')}
          </Button>
        }
      >
        <form
          id={`${id}-form`}
          onSubmit={(event) => {
            event.stopPropagation()
            void form.handleSubmit((input) => {
              form.clearErrors('root')
              setPending(input)
            })(event)
          }}
        >
          <fieldset disabled={save.isPending}>
            <FieldGroup>
              {(
                [
                  ['key', 'Scene identifier'],
                  ['name', 'Name'],
                  ['min_usd', 'Minimum amount (USD)'],
                  ['max_usd', 'Maximum amount (USD, 0 = unlimited)'],
                ] as const
              ).map(([field, label]) => (
                <Field key={field} data-invalid={!!errors[field]}>
                  <FieldLabel htmlFor={`${id}-${field}`}>{t(label)}</FieldLabel>
                  <Input
                    id={`${id}-${field}`}
                    aria-invalid={!!errors[field]}
                    {...form.register(field)}
                  />
                  <FieldError>
                    {errors[field]?.message && t(errors[field].message)}
                  </FieldError>
                </Field>
              ))}
              <Field>
                <FieldLabel htmlFor={`${id}-business`}>
                  {t('Business')}
                </FieldLabel>
                <NativeSelect
                  id={`${id}-business`}
                  {...form.register('business')}
                >
                  <NativeSelectOption value='wallet_topup'>
                    {t('Wallet top-up')}
                  </NativeSelectOption>
                </NativeSelect>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${id}-enabled`}>
                  {t('Enabled')}
                </FieldLabel>
                <Switch
                  id={`${id}-enabled`}
                  checked={form.watch('enabled')}
                  onCheckedChange={(enabled) =>
                    form.setValue('enabled', enabled, { shouldDirty: true })
                  }
                />
              </Field>
              {stringChoices.map((choice) => (
                <Field key={choice.field} data-invalid={!!errors[choice.field]}>
                  <FieldLabel htmlFor={`${id}-${choice.field}`}>
                    {t(choice.label)}
                  </FieldLabel>
                  <MultiSelect
                    id={`${id}-${choice.field}`}
                    placeholder={t(choice.label)}
                    options={choice.options}
                    selected={form.watch(choice.field)}
                    allowCreate={
                      choice.field === 'methods' ||
                      choice.field === 'currencies'
                    }
                    onChange={(values) => {
                      form.setValue(choice.field, values, {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                      if (
                        choice.field === 'providers' &&
                        !values.includes('adty_epay')
                      ) {
                        form.setValue('channel_ids', [], { shouldDirty: true })
                      }
                    }}
                  />
                  <FieldError>
                    {errors[choice.field]?.message &&
                      t(errors[choice.field]?.message ?? '')}
                  </FieldError>
                </Field>
              ))}
              {epaySelected && (
                <Field data-invalid={!!errors.channel_ids}>
                  <FieldLabel htmlFor={`${id}-channels`}>
                    {t('Allowed Epay channels')}
                  </FieldLabel>
                  <MultiSelect
                    id={`${id}-channels`}
                    placeholder={t('Allowed Epay channels')}
                    selected={form.watch('channel_ids').map(String)}
                    options={(channels.data ?? []).map((channel) => ({
                      value: String(channel.id),
                      label: `${channel.name} (#${channel.id})`,
                    }))}
                    onChange={(values) =>
                      form.setValue('channel_ids', values.map(Number), {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                    }
                  />
                  <FieldError>
                    {channels.isError
                      ? t('Failed to load data')
                      : errors.channel_ids?.message &&
                        t(errors.channel_ids.message)}
                  </FieldError>
                </Field>
              )}
              <Field>
                <FieldLabel htmlFor={`${id}-site-scope`}>
                  {t('Site scope')}
                </FieldLabel>
                <NativeSelect
                  id={`${id}-site-scope`}
                  {...form.register('site_scope', {
                    onChange: () =>
                      form.setValue('site_ids', [], { shouldDirty: true }),
                  })}
                >
                  {Object.entries(sceneAudienceLabels).map(([value, label]) => (
                    <NativeSelectOption key={value} value={value}>
                      {t(label)}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
              {selectedSites && (
                <Field data-invalid={!!errors.site_ids}>
                  <FieldLabel htmlFor={`${id}-sites`}>
                    {t('Selected sites')}
                  </FieldLabel>
                  <MultiSelect
                    id={`${id}-sites`}
                    placeholder={t('Selected sites')}
                    allowCreate
                    selected={form.watch('site_ids').map(String)}
                    options={(sites.data ?? [])
                      .filter((site) => site.site_id > 0)
                      .map((site) => ({
                        value: String(site.site_id),
                        label: `${site.site_name} (#${site.site_id})`,
                      }))}
                    onChange={(values) =>
                      form.setValue('site_ids', values.map(Number), {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                    }
                  />
                  <FieldError>
                    {sites.isError
                      ? t('Failed to load data')
                      : errors.site_ids?.message && t(errors.site_ids.message)}
                  </FieldError>
                </Field>
              )}
              {errors.root?.message && (
                <p role='alert' className='text-destructive text-sm'>
                  {errors.root.message}
                </p>
              )}
            </FieldGroup>
          </fieldset>
        </form>
      </Dialog>
      <ConfirmDialog
        open={!!pending}
        onOpenChange={(open) => !open && !save.isPending && setPending(null)}
        title={t('Save payment scene')}
        desc={t(
          'New orders will use these rules. Existing orders keep their original settings.'
        )}
        isLoading={save.isPending}
        handleConfirm={() => pending && save.mutate(pending)}
      />
      <DraftDiscardDialog
        open={discarding}
        onOpenChange={setDiscarding}
        onDiscard={props.onClose}
      />
    </>
  )
}
