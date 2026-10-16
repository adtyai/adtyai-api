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

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { createAdmin } from './api'
import {
  adminCreateErrorKey,
  buildAdminCreatePayload,
  EMPTY_ADMIN_FORM,
  validateAdminForm,
  type AdminCreateForm,
  type AdminFormErrors,
} from './lib/admins'

type FieldDef = {
  name: keyof AdminCreateForm
  label: string
  type: string
  autoComplete: string
  required?: boolean
}

const FIELDS: FieldDef[] = [
  {
    name: 'username',
    label: 'Username',
    type: 'text',
    autoComplete: 'off',
    required: true,
  },
  {
    name: 'password',
    label: 'Password',
    type: 'password',
    autoComplete: 'new-password',
    required: true,
  },
  {
    name: 'confirmPassword',
    label: 'Confirm password',
    type: 'password',
    autoComplete: 'new-password',
    required: true,
  },
  {
    name: 'displayName',
    label: 'Display name',
    type: 'text',
    autoComplete: 'off',
  },
  { name: 'email', label: 'Email', type: 'email', autoComplete: 'off' },
]

export function AdminCreateSettings() {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const [form, setForm] = useState<AdminCreateForm>(EMPTY_ADMIN_FORM)
  const [errors, setErrors] = useState<AdminFormErrors>({})
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    const errs = validateAdminForm(form)
    setErrors(errs)
    if (Object.keys(errs).length > 0) return
    setConfirmOpen(true)
  }

  async function onConfirm() {
    setSubmitting(true)
    try {
      const res = await createAdmin(buildAdminCreatePayload(form))
      if (res.ok) {
        toast.success(
          t('Administrator {{name}} created', {
            name: res.data?.username ?? form.username.trim(),
          })
        )
        setForm(EMPTY_ADMIN_FORM)
        setErrors({})
      } else {
        toast.error(t(adminCreateErrorKey(res.status, res.code)))
      }
    } catch {
      toast.error(t(adminCreateErrorKey()))
    } finally {
      setSubmitting(false)
      setConfirmOpen(false)
    }
  }

  if (!isRoot) {
    return (
      <div className='text-muted-foreground p-6'>
        {t('Only Root can create administrators')}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4 p-4 md:p-6'>
      <Card className='max-w-xl'>
        <CardHeader>
          <CardTitle>{t('Create administrator')}</CardTitle>
          <CardDescription>
            {t(
              'Root only. Creates an account with the Admin role (never Root). Requires a second confirmation.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className='flex flex-col gap-4' onSubmit={onSubmit} noValidate>
            {FIELDS.map((f) => (
              <div key={f.name} className='flex flex-col gap-1.5'>
                <Label htmlFor={`admin-create-${f.name}`}>
                  {t(f.label)}
                  {f.required ? (
                    <span className='text-destructive'> *</span>
                  ) : null}
                </Label>
                <Input
                  id={`admin-create-${f.name}`}
                  type={f.type}
                  autoComplete={f.autoComplete}
                  value={form[f.name]}
                  disabled={submitting}
                  aria-invalid={Boolean(errors[f.name])}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, [f.name]: e.target.value }))
                  }
                />
                {errors[f.name] ? (
                  <p className='text-destructive text-sm'>
                    {t(errors[f.name] as string)}
                  </p>
                ) : null}
              </div>
            ))}
            <div className='flex gap-2'>
              <Button type='submit' disabled={submitting}>
                {t('Create administrator')}
              </Button>
              <Button
                type='button'
                variant='outline'
                disabled={submitting}
                onClick={() => {
                  setForm(EMPTY_ADMIN_FORM)
                  setErrors({})
                }}
              >
                {t('Reset')}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!submitting) setConfirmOpen(open)
        }}
        title={t('Confirm creating administrator')}
        desc={t(
          'Create administrator "{{name}}" with the Admin role?',
          { name: form.username.trim() }
        )}
        confirmText={t('Confirm and create')}
        handleConfirm={() => void onConfirm()}
        isLoading={submitting}
      />
    </div>
  )
}
