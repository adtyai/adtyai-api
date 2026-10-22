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
import { RefreshCw, Save } from 'lucide-react'
import { useEffect, useMemo, useRef } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { USDT_CHAIN_CATALOG, type UsdtChainId } from '@/lib/usdt-chains'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import { useSystemOptions } from '../hooks/use-system-options'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  buildBepusdtUpdates,
  formatTokenFingerprint,
  parseBepusdtOptions,
  type BepusdtFormValues,
} from './lib/options'

const schema = z.object({
  BepusdtBaseURL: z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    try {
      const url = new URL(trimmed)
      return url.protocol === 'http:' || url.protocol === 'https:'
    } catch {
      return false
    }
  }, 'Provide a valid HTTP(S) base URL'),
  BepusdtApiToken: z.string(),
  BepusdtLegacyNotifyEnabled: z.boolean(),
  enabledChains: z.array(z.enum(['trc20', 'bep20', 'polygon', 'xlayer'])),
})

type FormValues = z.infer<typeof schema>

/**
 * Embeddable BEpusdt block for Payment Gateway billing section.
 * Same option keys/save path; drops page chrome and long copy.
 */
type BepusdtSectionProps = {
  /** When true, skip SettingsSection chrome (for Payment Gateway tab). */
  embedded?: boolean
}

export function BepusdtSection({ embedded = false }: BepusdtSectionProps = {}) {
  const { t } = useTranslation()
  const optionsQuery = useSystemOptions()
  const updateOption = useUpdateOption()

  const parsed = useMemo(
    () => parseBepusdtOptions(optionsQuery.data?.data),
    [optionsQuery.data?.data]
  )

  const initialRef = useRef<BepusdtFormValues>(parsed.form)
  initialRef.current = parsed.form

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<FormValues>,
    defaultValues: parsed.form,
  })

  useEffect(() => {
    form.reset(parsed.form)
  }, [parsed.form, form])

  const { isDirty, isSubmitting } = form.formState
  const saving = updateOption.isPending || isSubmitting

  async function onSubmit(values: FormValues) {
    const updates = buildBepusdtUpdates(values, initialRef.current)
    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    form.reset({
      ...values,
      BepusdtApiToken: '',
    })
  }

  const body = (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-end gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void optionsQuery.refetch()}
          disabled={optionsQuery.isFetching}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          size='sm'
          className='h-8'
          disabled={!isDirty || saving}
          onClick={() => void form.handleSubmit(onSubmit)()}
        >
          <Save className='size-3.5' />
          {saving ? t('Saving...') : t('Save Changes')}
        </Button>
      </div>

      {optionsQuery.isLoading ? (
        <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
          {t('Loading settings...')}
        </div>
      ) : null}

      {optionsQuery.isError && !optionsQuery.data ? (
        <ErrorState
          title={t('Failed to load settings')}
          onRetry={() => void optionsQuery.refetch()}
        />
      ) : null}

      {optionsQuery.data ? (
        <Form {...form}>
          <SettingsForm
            onSubmit={form.handleSubmit(onSubmit)}
            autoComplete='off'
            className='bg-card rounded-xl border p-5 shadow-xs'
          >
            <FormField
              control={form.control}
              name='BepusdtBaseURL'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('BEpusdt gateway URL')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='https://bepusdt.example.com'
                      autoComplete='off'
                      disabled={saving}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='BepusdtApiToken'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('API token (leave blank unless updating)')}
                  </FormLabel>
                  <div className='mb-2 flex flex-wrap items-center gap-2 text-xs'>
                    {parsed.token.secretSet ? (
                      <Badge variant='secondary'>{t('Secret set')}</Badge>
                    ) : (
                      <Badge variant='outline'>{t('Secret missing')}</Badge>
                    )}
                    <span className='text-muted-foreground font-mono'>
                      {t('Fingerprint')}:{' '}
                      {formatTokenFingerprint(parsed.token.fingerprint)}
                    </span>
                  </div>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      placeholder={t('Leave blank to keep current token')}
                      disabled={saving}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='space-y-2'>
              <div>
                <p className='text-sm font-medium'>
                  {t('Enabled USDT networks')}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Turn on the networks users can pay with. Amounts stay in USD.'
                  )}
                </p>
              </div>
              {USDT_CHAIN_CATALOG.map((chain) => (
                <FormField
                  key={chain.id}
                  control={form.control}
                  name='enabledChains'
                  render={({ field }) => {
                    const selected = field.value.includes(chain.id)
                    return (
                      <SettingsSwitchItem>
                        <SettingsSwitchContent>
                          <FormLabel>{chain.label}</FormLabel>
                          <p className='text-muted-foreground text-xs'>
                            {chain.network}
                          </p>
                        </SettingsSwitchContent>
                        <FormControl>
                          <Switch
                            checked={selected}
                            disabled={saving}
                            onCheckedChange={(on) => {
                              const next = on
                                ? [...field.value, chain.id]
                                : field.value.filter((id) => id !== chain.id)
                              const ordered = USDT_CHAIN_CATALOG.map(
                                (item) => item.id
                              ).filter((id) => next.includes(id as UsdtChainId))
                              field.onChange(ordered)
                            }}
                          />
                        </FormControl>
                      </SettingsSwitchItem>
                    )
                  }}
                />
              ))}
            </div>

            <FormField
              control={form.control}
              name='BepusdtLegacyNotifyEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Enable legacy BEpusdt notify path')}
                    </FormLabel>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Only for old callbacks. New orders do not use this. Leave it off.'
                      )}
                    </p>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={saving}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </SettingsForm>
        </Form>
      ) : null}
    </div>
  )

  if (embedded) {
    return (
      <div className='space-y-4'>
        <div>
          <h3 className='text-lg font-medium'>{t('BEpusdt gateway')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Configure BEpusdt (USDT) gateway via system options. API token is write-only; GET only returns secret_set and fingerprint. Saving does not probe connectivity.'
            )}
          </p>
        </div>
        {body}
      </div>
    )
  }

  return <SettingsSection title={t('BEpusdt gateway')}>{body}</SettingsSection>
}
