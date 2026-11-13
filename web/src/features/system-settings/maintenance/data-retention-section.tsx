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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsSection } from '../components/settings-section'

type RetentionCategory = {
  category: string
  title: string
  auto_cleanable: boolean
  configured: boolean
  retention_days: number | null
  action: string
  min_retention_days?: number | null
  allowed_actions: string[]
  default_behavior: string
  notes: string
}

type RetentionSettings = {
  categories: RetentionCategory[]
  login_security_env_default_days?: number
  usage_log_min_retention_days?: number
}

type Draft = {
  days: string
  action: string
}

const ACTION_LABELS: Record<string, string> = {
  delete: 'Delete after the period',
  deidentify: 'De-identify after the period',
  exclude: 'Permanent retention (not auto-cleaned)',
}

export function DataRetentionSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === ROLE.SUPER_ADMIN
  const settings = useQuery({
    queryKey: ['adty-data-retention'],
    queryFn: async () => {
      const response = await api.get('/api/adty/admin/data-retention')
      const body = response.data as {
        success?: boolean
        data?: RetentionSettings
        message?: string
      }
      if (!body.success || !body.data) {
        throw new Error(body.message || 'Failed to load data')
      }
      return body.data
    },
    enabled: isRoot,
  })
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})

  useEffect(() => {
    if (!settings.data?.categories) return
    const next: Record<string, Draft> = {}
    for (const category of settings.data.categories) {
      next[category.category] = {
        days:
          category.retention_days != null ? String(category.retention_days) : '',
        action: category.action || category.allowed_actions[0] || 'delete',
      }
    }
    setDrafts(next)
  }, [settings.data])

  const save = useMutation({
    mutationFn: async (policies: unknown) => {
      const response = await api.put('/api/adty/admin/data-retention', {
        policies,
      })
      const body = response.data as { success?: boolean; message?: string }
      if (!body.success) {
        throw new Error(body.message || 'Failed to save')
      }
    },
    onSuccess: async () => {
      toast.success(t('Retention settings saved. Later cleanup runs will use the new periods.'))
      await queryClient.invalidateQueries({ queryKey: ['adty-data-retention'] })
    },
    onError: (error) => {
      toast.error(getServerErrorMessage(error, t('Failed to save')))
    },
  })

  const categories = settings.data?.categories ?? []
  const canSave = useMemo(() => isRoot && categories.length > 0, [categories.length, isRoot])

  const saveCategory = (category: RetentionCategory) => {
    const draft = drafts[category.category]
    if (!draft) return
    if (!category.auto_cleanable) return
    const days = draft.days.trim()
    if (days === '') {
      save.mutate([
        {
          category: category.category,
          clear: true,
        },
      ])
      return
    }
    const parsed = Number(days)
    if (!Number.isInteger(parsed) || parsed < 1) {
      toast.error(t('A retention period must be a positive number of days'))
      return
    }
    save.mutate([
      {
        category: category.category,
        retention_days: parsed,
        action: draft.action,
      },
    ])
  }

  return (
    <SettingsSection title={t('Data retention')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Root configures how long each data category is kept. Unconfigured categories are not auto-deleted. Changing a period only affects later cleanup runs and does not immediately delete historical data. Notification history cannot be given an automatic cleanup period.'
        )}
      </p>
      {!isRoot ? (
        <p className='text-sm'>{t('Only Root can view and change data retention periods.')}</p>
      ) : settings.isError ? (
        <p className='text-destructive text-sm'>
          {getServerErrorMessage(settings.error, t('Failed to load data'))}
        </p>
      ) : (
        <div className='flex flex-col gap-5'>
          {categories.map((category) => {
            const draft = drafts[category.category] ?? {
              days: '',
              action: category.allowed_actions[0] || 'delete',
            }
            return (
              <div
                key={category.category}
                className='flex flex-col gap-2 border-b pb-4 last:border-b-0'
              >
                <div className='flex flex-col gap-1'>
                  <h4 className='font-medium'>{t(category.title)}</h4>
                  <p className='text-muted-foreground text-xs'>{t(category.notes)}</p>
                </div>
                {!category.auto_cleanable ? (
                  <p className='text-sm'>
                    {t(
                      'Permanent retention. Automatic cleanup tasks must not delete notification history. Only the user may manually delete their own deletable messages.'
                    )}
                  </p>
                ) : (
                  <div className='flex flex-wrap items-end gap-3'>
                    <div className='flex min-w-40 flex-col gap-1'>
                      <Label htmlFor={`retention-days-${category.category}`}>
                        {t('Retention period (days)')}
                      </Label>
                      <Input
                        id={`retention-days-${category.category}`}
                        inputMode='numeric'
                        placeholder={t('Not configured')}
                        value={draft.days}
                        onChange={(event) =>
                          setDrafts((current) => ({
                            ...current,
                            [category.category]: {
                              ...draft,
                              days: event.target.value,
                            },
                          }))
                        }
                      />
                    </div>
                    {category.allowed_actions.length > 1 && (
                      <div className='flex min-w-52 flex-col gap-1'>
                        <Label htmlFor={`retention-action-${category.category}`}>
                          {t('Action')}
                        </Label>
                        <select
                          id={`retention-action-${category.category}`}
                          className='border-input bg-background h-9 rounded-md border px-3 text-sm'
                          value={draft.action}
                          onChange={(event) =>
                            setDrafts((current) => ({
                              ...current,
                              [category.category]: {
                                ...draft,
                                action: event.target.value,
                              },
                            }))
                          }
                        >
                          {category.allowed_actions.map((action) => (
                            <option key={action} value={action}>
                              {t(ACTION_LABELS[action] ?? action)}
                            </option>
                          ))}
                        </select>
                      </div>
                    )}
                    <Button
                      type='button'
                      onClick={() => saveCategory(category)}
                      disabled={!canSave || save.isPending}
                    >
                      {t('Save')}
                    </Button>
                  </div>
                )}
                {category.min_retention_days ? (
                  <p className='text-muted-foreground text-xs'>
                    {t('Usage-log cleanup keeps a {{days}}-day safety floor.', {
                      days: category.min_retention_days,
                    })}
                  </p>
                ) : null}
                {!category.configured && category.auto_cleanable ? (
                  <p className='text-muted-foreground text-xs'>
                    {t('Not configured: the official cleanup task will not delete this category.')}
                  </p>
                ) : null}
              </div>
            )
          })}
        </div>
      )}
    </SettingsSection>
  )
}
