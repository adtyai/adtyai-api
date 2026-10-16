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
import { Info, RefreshCw, Save, Send, ShieldAlert } from 'lucide-react'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { settingKeyLabel } from '../business-params/lib/groups'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import {
  useAlertsConfig,
  usePutAlertsConfig,
  useTestAlerts,
} from './hooks/use-alerts'
import { alertKindLabelKey, alertThresholdLabelKey } from './lib/labels'
import {
  ALERTS_FORM_DEFAULTS,
  buildAlertsPut,
  configToForm,
  formatTokenFingerprint,
  isTelegramPendingConfigured,
  sortedKindKeys,
  sortedThresholdKeys,
} from './lib/put'
import type { AlertsFormValues, AlertsTestLevel } from './types'

const schema = z.object({
  bot_token: z.string(),
  urgent_chat_id: z.string(),
  pending_chat_id: z.string(),
  urgent_route_enabled: z.boolean(),
  pending_route_enabled: z.boolean(),
  kind_enabled: z.record(z.string(), z.boolean()),
  merge_minutes: z.coerce.number().int().min(1).max(60),
})

type FormValues = z.infer<typeof schema>

function isNotConfigured(err: unknown): boolean {
  if (err && typeof err === 'object' && 'code' in err) {
    return (err as { code?: string }).code === 'not_configured'
  }
  const cause =
    err instanceof Error && err.cause && typeof err.cause === 'object'
      ? (err.cause as { code?: string })
      : null
  return cause?.code === 'not_configured'
}

export function AlertsSettings() {
  const { t } = useTranslation()
  const alertsQuery = useAlertsConfig()
  const putAlerts = usePutAlertsConfig()
  const testMutation = useTestAlerts()
  const kindLabel = (kind: string) => {
    const labelKey = alertKindLabelKey(kind)
    return labelKey ? t(labelKey) : null
  }
  const thresholdLabel = (key: string) => {
    const labelKey = alertThresholdLabelKey(key)
    return labelKey ? t(labelKey) : null
  }

  const formDefaults = useMemo(
    () =>
      alertsQuery.data ? configToForm(alertsQuery.data) : ALERTS_FORM_DEFAULTS,
    [alertsQuery.data]
  )

  const initialRef = useRef<AlertsFormValues>(formDefaults)
  initialRef.current = formDefaults

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<FormValues>,
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const { isDirty, isSubmitting } = form.formState
  const saving = putAlerts.isPending || isSubmitting
  const kindEnabled = form.watch('kind_enabled')
  const kindKeys = useMemo(
    () => sortedKindKeys(kindEnabled ?? {}),
    [kindEnabled]
  )
  const thresholdKeys = useMemo(
    () => sortedThresholdKeys(alertsQuery.data?.thresholds ?? {}),
    [alertsQuery.data?.thresholds]
  )

  async function onSubmit(values: FormValues) {
    const body = buildAlertsPut(values, initialRef.current)
    if (!body) {
      toast.info(t('No changes to save'))
      return
    }

    try {
      const res = await putAlerts.mutateAsync(body)
      if (!res.success) {
        toast.error(getServerErrorMessage(res, t('Save failed')))
        return
      }
      toast.success(t('Saved'))
      form.reset({
        ...values,
        bot_token: '',
      })
    } catch (err) {
      toast.error(getServerErrorMessage(err, t('Save failed')))
    }
  }

  async function onTest(level: AlertsTestLevel) {
    try {
      await testMutation.mutateAsync({ level })
      toast.success(
        level === 'urgent'
          ? t('Urgent test alert sent')
          : t('Pending test alert sent')
      )
    } catch (err) {
      if (isNotConfigured(err)) {
        toast.error(
          t(
            'Alert bot is not configured. Set the Telegram bot token and the chat id for this level first.'
          )
        )
        return
      }
      toast.error(getServerErrorMessage(err, t('Test alert failed')))
    }
  }

  const isForbidden =
    (alertsQuery.error as { status?: number } | null)?.status === 403

  return (
    <SectionPageLayout stackActionsOnMobile>
      <SectionPageLayout.Title>{t('Telegram alerts')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void alertsQuery.refetch()}
          disabled={alertsQuery.isFetching}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          disabled={testMutation.isPending || saving}
          onClick={() => void onTest('urgent')}
        >
          <Send className='size-3.5' />
          {t('Send urgent test')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          disabled={testMutation.isPending || saving}
          onClick={() => void onTest('pending')}
        >
          <Send className='size-3.5' />
          {t('Send pending test')}
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
              'Configure Telegram alert routing for ops. The Telegram bot token is write-only; only whether it is set and its fingerprint are shown. Does not affect the Telegram login token.'
            )}
          </p>

          <div className='grid gap-3 md:grid-cols-2'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Most thresholds are owned by Business parameters. This page only edits AlertMergeMinutes; the table below is read-only.'
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
                  'Uses a dedicated Telegram bot token for alerts, separate from the Telegram login token.'
                )}
              </div>
            </div>
          </div>

          {alertsQuery.data ? (
            <div className='flex flex-wrap items-start gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div className='min-w-0 flex-1 space-y-1'>
                <div className='font-medium'>
                  {t('Daily report push time (HH:mm, Asia/Shanghai)')}
                </div>
                <p>
                  <span className='font-mono text-xs'>
                    {t('Daily report time')}{' '}
                    {alertsQuery.data.thresholds?.DailyReportTime || '09:00'}
                  </span>
                  {' · '}
                  {t('Timezone is Asia/Shanghai. Format HH:mm (e.g. 09:00).')}
                </p>
                <p>
                  {t(
                    'Daily report is sent via the Telegram pending alert channel.'
                  )}{' '}
                  <Link
                    to='/system-settings/business-params'
                    className='font-medium underline underline-offset-2'
                  >
                    {t('Open business parameters')}
                  </Link>
                  {' · '}
                  {isTelegramPendingConfigured(alertsQuery.data) ? (
                    <Badge variant='secondary'>
                      {t('Telegram pending channel configured')}
                    </Badge>
                  ) : (
                    <Badge variant='outline'>
                      {t('Telegram pending channel not configured')}
                    </Badge>
                  )}
                </p>
              </div>
            </div>
          ) : null}

          {alertsQuery.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading settings...')}
            </div>
          ) : null}

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'Admin permission is required to manage Telegram alerts.'
              )}
            />
          ) : null}

          {alertsQuery.isError && !alertsQuery.data && !isForbidden ? (
            <ErrorState
              title={t('Failed to load settings')}
              onRetry={() => void alertsQuery.refetch()}
            />
          ) : null}

          {alertsQuery.data ? (
            <Form {...form}>
              <SettingsForm
                onSubmit={form.handleSubmit(onSubmit)}
                autoComplete='off'
                className='bg-card space-y-6 rounded-xl border p-5 shadow-xs'
              >
                <FormField
                  control={form.control}
                  name='bot_token'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Bot token (leave blank unless updating)')}
                      </FormLabel>
                      <div className='mb-2 flex flex-wrap items-center gap-2 text-xs'>
                        {alertsQuery.data.bot_token.secret_set ? (
                          <Badge variant='secondary'>{t('Secret set')}</Badge>
                        ) : (
                          <Badge variant='outline'>{t('Secret missing')}</Badge>
                        )}
                        <span className='text-muted-foreground'>
                          {t(
                            settingKeyLabel(
                              alertsQuery.data.bot_token.option_key
                            )
                          )}
                        </span>
                        <span className='text-muted-foreground'>
                          {t('Fingerprint')}:{' '}
                          {alertsQuery.data.bot_token.fingerprint ? (
                            <span className='font-mono'>
                              {formatTokenFingerprint(
                                alertsQuery.data.bot_token.fingerprint
                              )}
                            </span>
                          ) : (
                            t('Not set')
                          )}
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

                <div className='grid gap-4 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='urgent_chat_id'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Urgent chat ID')}</FormLabel>
                        <FormControl>
                          <Input
                            autoComplete='off'
                            placeholder='-100xxxxxxxxxx'
                            disabled={saving}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Telegram chat/channel id for urgent alerts.')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='pending_chat_id'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Pending chat ID')}</FormLabel>
                        <FormControl>
                          <Input
                            autoComplete='off'
                            placeholder='-100xxxxxxxxxx'
                            disabled={saving}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Telegram chat/channel id for pending alerts.')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='urgent_route_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Enable urgent route')}</FormLabel>
                        <FormDescription>
                          {t(
                            'When off, urgent-level alerts are not delivered.'
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

                <FormField
                  control={form.control}
                  name='pending_route_enabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Enable pending route')}</FormLabel>
                        <FormDescription>
                          {t(
                            'When off, pending-level alerts are not delivered.'
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

                <FormField
                  control={form.control}
                  name='merge_minutes'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Merge window (minutes)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={1}
                          max={60}
                          autoComplete='off'
                          disabled={saving}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'AlertMergeMinutes (1–60). Same key as business parameters; editable here too.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className='space-y-3'>
                  <div>
                    <h3 className='text-sm font-medium'>
                      {t('Alert kinds enabled')}
                    </h3>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Toggle which alert categories may fire. Unchecked kinds are suppressed.'
                      )}
                    </p>
                  </div>
                  <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3'>
                    {kindKeys.map((key) => (
                      <FormField
                        key={key}
                        control={form.control}
                        name={`kind_enabled.${key}` as const}
                        render={({ field }) => (
                          <SettingsSwitchItem className='rounded-lg border px-3 py-2'>
                            <SettingsSwitchContent>
                              {kindLabel(key) ? (
                                <FormLabel className='text-xs' title={key}>
                                  {kindLabel(key)}
                                </FormLabel>
                              ) : (
                                <FormLabel className='font-mono text-xs'>
                                  {key}
                                </FormLabel>
                              )}
                            </SettingsSwitchContent>
                            <FormControl>
                              <Switch
                                checked={Boolean(field.value)}
                                onCheckedChange={field.onChange}
                                disabled={saving}
                              />
                            </FormControl>
                          </SettingsSwitchItem>
                        )}
                      />
                    ))}
                  </div>
                </div>

                <div className='space-y-3'>
                  <div>
                    <h3 className='text-sm font-medium'>
                      {t('Thresholds (read-only)')}
                    </h3>
                    <p className='text-muted-foreground text-xs'>
                      {/* 后端 thresholds_note 是英文开发说明，按总管定不显示 */}
                      {t(
                        'Thresholds live in business parameters; shown here for reference.'
                      )}
                    </p>
                  </div>
                  <div className='overflow-hidden rounded-lg border'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Key')}</TableHead>
                          <TableHead>{t('Value')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {thresholdKeys.length === 0 ? (
                          <TableRow>
                            <TableCell
                              colSpan={2}
                              className='text-muted-foreground text-sm'
                            >
                              {t('No thresholds')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          thresholdKeys.map((key) => (
                            <TableRow key={key}>
                              {thresholdLabel(key) ? (
                                <TableCell className='text-xs' title={key}>
                                  {thresholdLabel(key)}
                                </TableCell>
                              ) : (
                                <TableCell className='font-mono text-xs'>
                                  {key}
                                </TableCell>
                              )}
                              <TableCell className='font-mono text-xs'>
                                {alertsQuery.data?.thresholds?.[key] ?? ''}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </div>
              </SettingsForm>
            </Form>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
