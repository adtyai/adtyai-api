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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Form } from '@/components/ui/form'
import { PromoPanel } from '@/features/adty-promo/components/promo-panel'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  SettingsForm,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useDraftNavigation } from '../hooks/use-draft-navigation'
import { useUpdateOption } from '../hooks/use-update-option'

export function AdtyPromoSettingsSection(props: { defaultEnabled: boolean }) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const canWrite = (user?.role ?? 0) >= ROLE.SUPER_ADMIN
  const form = useForm({ defaultValues: { enabled: props.defaultEnabled } })
  const update = useUpdateOption()
  const { isDirty, isSubmitting } = form.formState
  useDraftNavigation(isDirty)

  const save = form.handleSubmit(async (values) => {
    if ((useAuthStore.getState().auth.user?.role ?? 0) < ROLE.SUPER_ADMIN) {
      return
    }
    try {
      await update.mutateAsync({
        key: 'adty_marketing_setting.promo_enabled',
        value: String(values.enabled),
      })
      form.reset(values)
    } catch (error) {
      handleServerError(error)
    }
  })

  return (
    <SettingsSection title={t('Promo codes')}>
      <Form {...form}>
        <SettingsForm onSubmit={save}>
          <SettingsPageFormActions
            onSave={save}
            isSaving={update.isPending || isSubmitting}
            isSaveDisabled={!canWrite || !isDirty}
          />
          <SettingsSwitchField
            controlId='promo-enabled'
            label={t('Allow users to enter promo codes')}
            checked={form.watch('enabled')}
            onCheckedChange={(enabled) =>
              form.setValue('enabled', enabled, { shouldDirty: true })
            }
            disabled={!canWrite || update.isPending || isSubmitting}
          />
        </SettingsForm>
      </Form>
      <PromoPanel canCreate={canWrite} canWrite={canWrite} />
    </SettingsSection>
  )
}
