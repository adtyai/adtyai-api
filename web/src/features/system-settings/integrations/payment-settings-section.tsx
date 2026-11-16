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
import { useLocation, useNavigate } from '@tanstack/react-router'
import { Code2, Eye, ShieldAlert } from 'lucide-react'
import * as React from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PaymentChannels } from '@/features/adty-payment-channels'
import { PaymentSceneManager } from '@/features/adty-payment-scenes'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import { AdtyBonusRatesEditor } from './adty-bonus-rates-editor'
import {
  AdtyEpaySettingsSection,
  type AdtyEpaySettingsDefaults,
} from './adty-epay-settings-section'
import { AmountOptionsVisualEditor } from './amount-options-visual-editor'
import {
  BEpusdtSettingsSection,
  type BEpusdtSettingsDefaults,
} from './bepusdt-settings-section'
import {
  formatJsonForEditor,
  getJsonError,
  normalizeJsonForComparison,
  removeTrailingSlash,
} from './utils'

function isHttpOriginUrl(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return true

  try {
    const url = new URL(trimmed)
    const isHttpProtocol = url.protocol === 'http:' || url.protocol === 'https:'
    const hasNoPath = url.pathname === '' || url.pathname === '/'
    return isHttpProtocol && hasNoPath && !url.search && !url.hash
  } catch {
    return false
  }
}

const paymentSchema = z.object({
  Price: z.coerce.number().min(0),
  MinTopUp: z.coerce.number().min(0),
  CustomCallbackAddress: z
    .string()
    .refine(
      isHttpOriginUrl,
      'Enter only a top-level callback domain, for example https://api.example.com, without any path.'
    ),
  AmountOptions: z.string().superRefine((value, ctx) => {
    const error = getJsonError(value, (parsed) => Array.isArray(parsed))
    if (error) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: error,
      })
    }
  }),
  BonusRates: z.string().superRefine((value, ctx) => {
    const error = getJsonError(
      value,
      (parsed) =>
        !!parsed && typeof parsed === 'object' && !Array.isArray(parsed)
    )
    if (error) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: error,
      })
    }
  }),
})

type PaymentFormValues = z.infer<typeof paymentSchema>
const providerSchemas = {
  general: paymentSchema.pick({
    Price: true,
    MinTopUp: true,
    AmountOptions: true,
    BonusRates: true,
  }),
  epay: paymentSchema.pick({
    CustomCallbackAddress: true,
  }),
}
type PaymentTab = keyof typeof providerSchemas | 'usdt'
const PAYMENT_TABS = ['general', 'epay', 'usdt'] as const

function isPaymentTab(tab: string): tab is PaymentTab {
  return (PAYMENT_TABS as readonly string[]).includes(tab)
}

function paymentTabSchema(tab: PaymentTab) {
  return providerSchemas[tab === 'usdt' ? 'general' : tab]
}

const paymentTabContentClassName = 'mt-6 min-w-0'

type PaymentSettingsSectionProps = {
  defaultValues: PaymentFormValues
  epayDefaultValues: AdtyEpaySettingsDefaults
  usdtDefaultValues: BEpusdtSettingsDefaults
}

export function PaymentSettingsSection({
  defaultValues,
  epayDefaultValues,
  usdtDefaultValues,
}: PaymentSettingsSectionProps) {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate({ from: '/system-settings/billing/$section' })
  const initialTab = React.useMemo(() => {
    const tab = new URLSearchParams(location.searchStr).get('tab')
    return tab && isPaymentTab(tab) ? tab : 'general'
  }, [location.searchStr])
  const [activeTab, setActiveTab] = React.useState<PaymentTab>(initialTab)
  const updateOption = useUpdateOption()
  const initialRef = React.useRef(defaultValues)
  const defaultsSignature = React.useMemo(
    () => JSON.stringify(defaultValues),
    [defaultValues]
  )

  const [amountOptionsVisualMode, setAmountOptionsVisualMode] =
    React.useState(true)

  React.useEffect(() => {
    setActiveTab(initialTab)
  }, [initialTab])

  const form = useForm<PaymentFormValues>({
    resolver: zodResolver(
      paymentTabSchema(activeTab).passthrough()
    ) as unknown as Resolver<PaymentFormValues>,
    mode: 'onChange', // Enable real-time validation
    defaultValues: {
      ...defaultValues,
      AmountOptions: formatJsonForEditor(defaultValues.AmountOptions),
      BonusRates: formatJsonForEditor(defaultValues.BonusRates),
    },
  })

  const { isSubmitting } = form.formState

  React.useEffect(() => {
    const parsedDefaults = JSON.parse(defaultsSignature) as PaymentFormValues
    initialRef.current = parsedDefaults
    form.reset(
      {
        ...parsedDefaults,
        AmountOptions: formatJsonForEditor(parsedDefaults.AmountOptions),
        BonusRates: formatJsonForEditor(parsedDefaults.BonusRates),
      },
      { keepDirtyValues: true }
    )
  }, [defaultsSignature, form])

  const onSubmit = async (values: PaymentFormValues) => {
    const sanitized = {
      Price: values.Price,
      MinTopUp: values.MinTopUp,
      CustomCallbackAddress: removeTrailingSlash(values.CustomCallbackAddress),
      AmountOptions: values.AmountOptions.trim(),
      BonusRates: values.BonusRates.trim(),
    }

    const initial = {
      Price: initialRef.current.Price,
      MinTopUp: initialRef.current.MinTopUp,
      CustomCallbackAddress: removeTrailingSlash(
        initialRef.current.CustomCallbackAddress
      ),
      AmountOptions: initialRef.current.AmountOptions.trim(),
      BonusRates: initialRef.current.BonusRates.trim(),
    }

    const updates: Array<{ key: string; value: string | number | boolean }> = []

    if (sanitized.Price !== initial.Price) {
      updates.push({ key: 'Price', value: sanitized.Price })
    }

    if (sanitized.MinTopUp !== initial.MinTopUp) {
      updates.push({ key: 'MinTopUp', value: sanitized.MinTopUp })
    }

    if (sanitized.CustomCallbackAddress !== initial.CustomCallbackAddress) {
      updates.push({
        key: 'CustomCallbackAddress',
        value: sanitized.CustomCallbackAddress,
      })
    }

    if (
      normalizeJsonForComparison(sanitized.AmountOptions) !==
      normalizeJsonForComparison(initial.AmountOptions)
    ) {
      updates.push({
        key: 'payment_setting.amount_options',
        value: sanitized.AmountOptions,
      })
    }

    if (
      normalizeJsonForComparison(sanitized.BonusRates) !==
      normalizeJsonForComparison(initial.BonusRates)
    ) {
      updates.push({
        key: 'adty_topup_setting.bonus_rates',
        value: sanitized.BonusRates,
      })
    }

    const fields = paymentTabSchema(activeTab).shape
    const tabUpdates = updates.filter((update) => {
      if (
        update.key === 'payment_setting.amount_options' ||
        update.key === 'adty_topup_setting.bonus_rates'
      ) {
        return activeTab === 'general'
      }
      return update.key in fields
    })

    if (tabUpdates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of tabUpdates) {
      await updateOption.mutateAsync(update)
      let field = update.key
      if (update.key === 'payment_setting.amount_options') {
        field = 'AmountOptions'
      } else if (update.key === 'adty_topup_setting.bonus_rates') {
        field = 'BonusRates'
      }
      if (field in initialRef.current) {
        initialRef.current[field as keyof PaymentFormValues] =
          update.value as never
      }
    }
  }

  return (
    <SettingsSection title={t('Payment Gateway')}>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(onSubmit)}
          className='gap-y-8'
          data-no-autosubmit='true'
        >
          {activeTab !== 'usdt' && (
            <SettingsPageFormActions
              onSave={form.handleSubmit(onSubmit)}
              isSaving={updateOption.isPending || isSubmitting}
            />
          )}
          <Tabs
            value={activeTab}
            onValueChange={(value) => {
              setActiveTab(value as PaymentTab)
              void navigate({
                search: (previous) => ({
                  ...previous,
                  tab: value === 'general' ? undefined : value,
                }),
              })
            }}
            className='min-w-0'
          >
            <div className='overflow-x-auto pb-1'>
              <TabsList className='grid min-w-[24rem] grid-cols-3'>
                <TabsTrigger value='general'>{t('General')}</TabsTrigger>
                <TabsTrigger value='epay'>Epay</TabsTrigger>
                <TabsTrigger value='usdt'>USDT</TabsTrigger>
              </TabsList>
            </div>

            <TabsContent value='general' className={paymentTabContentClassName}>
              <div className='space-y-4'>
                <PaymentSceneManager />
                <div>
                  <h3 className='text-lg font-medium'>
                    {t('General Settings')}
                  </h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Shared configuration for all payment gateways')}
                  </p>
                </div>

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='Price'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Price (local currency / USD)')}
                        </FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            step='0.01'
                            min={0}
                            {...safeNumberFieldProps(field)}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'How much to charge for each US dollar of balance (Epay)'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='MinTopUp'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Minimum top-up (USD)')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            step='0.01'
                            min={0}
                            {...safeNumberFieldProps(field)}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Smallest USD amount users can recharge (Epay)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className='grid gap-6 md:grid-cols-2 md:items-start'>
                  <FormField
                    control={form.control}
                    name='AmountOptions'
                    render={({ field }) => (
                      <FormItem>
                        <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                          <FormLabel>{t('Top-up amount options')}</FormLabel>
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              setAmountOptionsVisualMode(
                                !amountOptionsVisualMode
                              )
                            }
                            className='w-full sm:w-auto'
                          >
                            {amountOptionsVisualMode ? (
                              <>
                                <Code2 className='mr-2 h-3 w-3' />
                                {t('JSON Editor')}
                              </>
                            ) : (
                              <>
                                <Eye className='mr-2 h-3 w-3' />
                                {t('Visual Editor')}
                              </>
                            )}
                          </Button>
                        </div>
                        <FormControl>
                          {amountOptionsVisualMode ? (
                            <AmountOptionsVisualEditor
                              value={field.value}
                              onChange={field.onChange}
                            />
                          ) : (
                            <JsonCodeEditor
                              value={field.value}
                              onChange={field.onChange}
                              name={field.name}
                              onBlur={field.onBlur}
                              textareaRef={field.ref}
                              placeholder='[10, 20, 50, 100]'
                              heightClassName='h-40 min-h-40 max-h-40'
                              aria-invalid={Boolean(
                                form.formState.errors.AmountOptions
                              )}
                            />
                          )}
                        </FormControl>
                        <FormDescription>
                          {t('Preset recharge amounts (JSON array)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='BonusRates'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Top-up bonus')}</FormLabel>
                        <FormControl>
                          <AdtyBonusRatesEditor
                            value={field.value}
                            onChange={field.onChange}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Bonus percent credited on top of each top-up. The bonus earns no commission.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent
              value='usdt'
              keepMounted
              className={paymentTabContentClassName}
            >
              <BEpusdtSettingsSection
                defaultValues={usdtDefaultValues}
                inlineActions
              />
            </TabsContent>

            <TabsContent
              value='epay'
              keepMounted
              className={paymentTabContentClassName}
            >
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>{t('Epay Gateway')}</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Configuration for Epay payment integration')}
                  </p>
                </div>

                <Alert>
                  <ShieldAlert className='h-4 w-4' />
                  <AlertTitle>{t('Epay safety reminder')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Epay is a payment protocol, not a specific official website. Verify the provider yourself and do not trust random third-party Epay deployments.'
                    )}
                  </AlertDescription>
                </Alert>

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='CustomCallbackAddress'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Callback address')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('https://gateway.example.com')}
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Only enter the site origin, for example https://api.example.com. Do not include any path such as /api/user/epay/notify. Leave blank to use the server address.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className='space-y-8'>
                  <AdtyEpaySettingsSection
                    defaultValues={epayDefaultValues}
                    inlineActions
                  />
                  <PaymentChannels />
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
