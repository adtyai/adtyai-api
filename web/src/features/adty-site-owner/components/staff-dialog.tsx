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
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  resetStaffPassword,
  saveStaffAccount,
  type StaffAccount,
  type StaffPermission,
  type StaffRole,
} from '@/features/adty-site-staff/api'
import { staffRolePermissions } from '@/features/adty-site-staff/lib/permissions'

import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'

export function StaffDialog(props: {
  siteId: number
  staff: StaffAccount | null
  mode: 'create' | 'edit' | 'password'
  onClose: () => void
}) {
  const { t } = useTranslation()
  const schema = z
    .object({
      username: z.string().max(20),
      display_name: z.string().min(1, t('Please enter a display name')).max(20),
      password: z
        .string()
        .max(128)
        .refine(
          (value) => props.mode === 'edit' || [...value].length >= 15,
          t('Staff passwords must contain between 15 and 128 characters.')
        ),
      role: z.enum(['support', 'finance', 'operations', 'manager']),
      permissions: z.array(
        z.enum([
          'customers.read',
          'customers.balance',
          'customers.topups',
          'customers.usage',
          'customers.password',
          'customers.disable',
        ])
      ),
    })
    .refine(
      (value) =>
        props.mode !== 'create' ||
        (value.username.trim().length > 0 && !value.username.includes('@')),
      { path: ['username'], message: t('Please enter a valid username') }
    )
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      username: props.staff?.username ?? '',
      display_name: props.staff?.display_name ?? '',
      password: '',
      role: props.staff?.role ?? 'support',
      permissions: props.staff?.permissions ?? ['customers.read'],
    },
  })
  let action: 'staff.password' | 'staff.create' | 'staff.update' =
    'staff.update'
  if (props.mode === 'password') action = 'staff.password'
  if (props.mode === 'create') action = 'staff.create'
  const save = useVerifiedOwnerMutation<Values>({
    action,
    target: () => props.staff?.id ?? props.siteId,
    run: async (values, proof) => {
      if (props.mode === 'password' && props.staff) {
        return resetStaffPassword(props.staff, values.password, proof)
      }
      const input = {
        display_name: values.display_name,
        role: values.role,
        permissions: values.permissions,
        status: props.staff?.status ?? 'enabled',
        expected_version: props.staff?.version,
      }
      if (props.mode === 'create') {
        return saveStaffAccount(
          null,
          { ...input, username: values.username, password: values.password },
          proof
        )
      }
      return saveStaffAccount(props.staff?.id ?? null, input, proof)
    },
    onSuccess: props.onClose,
  })
  const permissionOptions: { value: StaffPermission; label: string }[] = [
    { value: 'customers.read', label: t('View customer list') },
    { value: 'customers.balance', label: t('View customer balances') },
    { value: 'customers.topups', label: t('View customer recharges') },
    { value: 'customers.usage', label: t('View customer usage') },
    {
      value: 'customers.password',
      label: t('Send password reset instructions'),
    },
    { value: 'customers.disable', label: t('Disable customer') },
  ]
  let title = t('Edit staff account')
  if (props.mode === 'create') title = t('Create staff account')
  if (props.mode === 'password') title = t('Reset staff password')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose()
      }}
      title={title}
      description={t(
        'Staff receive only the permissions selected here. Wallets, withdrawals, API keys and cross-site access are always blocked.'
      )}
      footer={
        <>
          <Button
            variant='outline'
            disabled={save.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='staff-account-form'
            disabled={save.isPending}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id='staff-account-form'
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
      >
        <FieldGroup>
          {props.mode === 'create' && (
            <Field data-invalid={!!form.formState.errors.username}>
              <FieldLabel htmlFor='staff-username'>{t('Username')}</FieldLabel>
              <Input
                id='staff-username'
                autoComplete='off'
                aria-invalid={!!form.formState.errors.username}
                {...form.register('username')}
              />
              <FieldError errors={[form.formState.errors.username]} />
            </Field>
          )}
          {props.mode !== 'password' && (
            <Field data-invalid={!!form.formState.errors.display_name}>
              <FieldLabel htmlFor='staff-name'>{t('Display name')}</FieldLabel>
              <Input
                id='staff-name'
                autoComplete='off'
                aria-invalid={!!form.formState.errors.display_name}
                {...form.register('display_name')}
              />
              <FieldError errors={[form.formState.errors.display_name]} />
            </Field>
          )}
          {props.mode !== 'edit' && (
            <Field data-invalid={!!form.formState.errors.password}>
              <FieldLabel htmlFor='staff-password'>{t('Password')}</FieldLabel>
              <Input
                id='staff-password'
                type='password'
                autoComplete='new-password'
                aria-invalid={!!form.formState.errors.password}
                {...form.register('password')}
              />
              <FieldError errors={[form.formState.errors.password]} />
            </Field>
          )}
          {props.mode !== 'password' && (
            <>
              <Field>
                <FieldLabel htmlFor='staff-role'>{t('Staff role')}</FieldLabel>
                <NativeSelect
                  id='staff-role'
                  {...form.register('role', {
                    onChange: (event) =>
                      form.setValue(
                        'permissions',
                        staffRolePermissions(event.target.value as StaffRole),
                        { shouldDirty: true }
                      ),
                  })}
                >
                  <NativeSelectOption value='support'>
                    {t('Customer support')}
                  </NativeSelectOption>
                  <NativeSelectOption value='finance'>
                    {t('Finance')}
                  </NativeSelectOption>
                  <NativeSelectOption value='operations'>
                    {t('Operations')}
                  </NativeSelectOption>
                  <NativeSelectOption value='manager'>
                    {t('Management')}
                  </NativeSelectOption>
                </NativeSelect>
              </Field>
              <Controller
                control={form.control}
                name='permissions'
                render={({ field }) => (
                  <FieldGroup>
                    {permissionOptions.map((option) => (
                      <Field key={option.value} orientation='horizontal'>
                        <Checkbox
                          id={`staff-${option.value}`}
                          checked={field.value.includes(option.value)}
                          onCheckedChange={(checked) =>
                            field.onChange(
                              checked
                                ? [...field.value, option.value]
                                : field.value.filter(
                                    (value) => value !== option.value
                                  )
                            )
                          }
                        />
                        <FieldLabel htmlFor={`staff-${option.value}`}>
                          {option.label}
                        </FieldLabel>
                      </Field>
                    ))}
                  </FieldGroup>
                )}
              />
            </>
          )}
        </FieldGroup>
      </form>
    </Dialog>
  )
}
