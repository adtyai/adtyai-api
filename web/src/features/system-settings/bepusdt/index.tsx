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
import { Link } from '@tanstack/react-router'
import { Info, RefreshCw, Save, ShieldAlert } from 'lucide-react'
import { useEffect, useMemo, useRef } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { useSystemOptions } from '../hooks/use-system-options'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  BEPUSDT_DEFAULTS,
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
  BepusdtTradeType: z.string().min(1),
  BepusdtCurrency: z.string().min(1),
})

type FormValues = z.infer<typeof schema>

export function BepusdtSettings() {
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
  const legacyEnabled = form.watch('BepusdtLegacyNotifyEnabled')
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

    // Clear token input after a successful write so we never re-display it.
    form.reset({
      ...values,
      BepusdtApiToken: '',
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('BEpusdt gateway')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
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
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Configure BEpusdt (USDT) gateway via system options. API token is write-only; GET only returns secret_set and fingerprint. Saving does not probe connectivity.'
            )}
          </p>

          <div className='grid gap-3 md:grid-cols-2'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Order timeout and underpay tolerance live on Business parameters (BepusdtOrderTimeoutMinutes / UsdtUnderpayTolerance).'
                )}{' '}
                <Link
                  to='/system-settings/business-params'
                  className='font-medium underline underline-offset-2'
                >
                  {t('Open business parameters')}
                </Link>
              </div>
            </div>
            <div className='flex gap-2.5 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-[13px] leading-5 text-amber-950 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100'>
              <ShieldAlert className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Legacy notify path is off by default. Enabling it does not auto-test connectivity.'
                )}
              </div>
            </div>
          </div>

          {optionsQuery.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
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
                      <FormLabel>{t('BEpusdt base URL')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder='https://bepusdt.example.com'
                          autoComplete='off'
                          disabled={saving}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Gateway API origin. Trailing slashes are stripped on save.'
                        )}
                      </FormDescription>
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
                      <FormDescription>
                        {t(
                          'Never re-displayed after save; only fingerprint is shown. Empty value does not overwrite.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='BepusdtTradeType'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Trade type')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={BEPUSDT_DEFAULTS.BepusdtTradeType}
                          autoComplete='off'
                          disabled={saving}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Default usdt.trc20. Stored lowercased.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='BepusdtCurrency'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Currency')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={BEPUSDT_DEFAULTS.BepusdtCurrency}
                          autoComplete='off'
                          disabled={saving}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Order amount currency (default USD). Stored uppercased.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='BepusdtLegacyNotifyEnabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>
                          {t('Enable legacy BEpusdt notify path')}
                        </FormLabel>
                        <FormDescription>
                          {legacyEnabled
                            ? t(
                                'Legacy callback path is ON. Prefer the new notify route unless you must keep old clients.'
                              )
                            : t(
                                'Legacy callback path stays OFF by default. Turning it on does not run a connectivity probe.'
                              )}
                        </FormDescription>
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
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
