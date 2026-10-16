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

import { Plus, RefreshCw, Save } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ROLE } from '@/lib/roles'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  SettingsSwitchContent,
  SettingsSwitchRow,
} from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import {
  useMenuVisibility,
  usePutMenuVisibility,
} from './hooks/use-menu-visibility'
import { knownKeysFor } from './lib/keys'
import {
  addCustomKey,
  buildMenuVisibilityPut,
  sideIsDirty,
  viewToDrafts,
} from './lib/put'
import type {
  MenuVisibilityPutInput,
  MenuVisibilitySide,
  SideDraft,
} from './types'

const EMPTY_DRAFTS = viewToDrafts({ user: {}, admin: {} })

/**
 * Embeddable menu-visibility block for the Sidebar modules site section.
 * Root/SUPER_ADMIN only; non-root sees nothing (sibling modules stay usable).
 */
export function MenuVisibilitySection() {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const query = useMenuVisibility({ enabled: isRoot })
  const putMutation = usePutMenuVisibility()

  const [userDraft, setUserDraft] = useState<SideDraft>(EMPTY_DRAFTS.user)
  const [adminDraft, setAdminDraft] = useState<SideDraft>(EMPTY_DRAFTS.admin)
  const [initialUser, setInitialUser] = useState<SideDraft>(EMPTY_DRAFTS.user)
  const [initialAdmin, setInitialAdmin] = useState<SideDraft>(
    EMPTY_DRAFTS.admin
  )
  const [tab, setTab] = useState<MenuVisibilitySide>('user')
  const [customInput, setCustomInput] = useState('')

  useEffect(() => {
    if (!query.data) return
    const drafts = viewToDrafts(query.data)
    setUserDraft(drafts.user)
    setAdminDraft(drafts.admin)
    setInitialUser(drafts.user)
    setInitialAdmin(drafts.admin)
  }, [query.data])

  // Hooks above must run unconditionally; gate render after.
  if (!isRoot) {
    return null
  }

  const dirty =
    sideIsDirty(userDraft, initialUser) || sideIsDirty(adminDraft, initialAdmin)
  const saving = putMutation.isPending

  async function doPut(body: MenuVisibilityPutInput) {
    try {
      const res = await putMutation.mutateAsync(body)
      if (!res.success) {
        if (res.code === 'forbidden' || res.code === 'root_required') {
          toast.error(
            t('Root permission is required to manage menu visibility.')
          )
          return
        }
        toast.error(getServerErrorMessage(res, t('Save failed')))
        return
      }
      toast.success(t('Saved'))
      void query.refetch()
    } catch (err) {
      const status = (err as { status?: number })?.status
      const code = (err as { code?: string } | null)?.code
      if (status === 403 || code === 'root_required') {
        toast.error(
          t('Root permission is required to manage menu visibility.')
        )
        return
      }
      toast.error(getServerErrorMessage(err, t('Save failed')))
    }
  }

  async function onSaveClick() {
    if (!isRoot) {
      toast.error(t('Root permission is required to manage menu visibility.'))
      return
    }
    const body = buildMenuVisibilityPut(
      { user: userDraft, admin: adminDraft },
      { user: initialUser, admin: initialAdmin }
    )
    if (!body) {
      toast.info(t('No changes to save'))
      return
    }
    await doPut(body)
  }

  function setSideValue(side: MenuVisibilitySide, key: string, checked: boolean) {
    const setter = side === 'user' ? setUserDraft : setAdminDraft
    setter((prev) => ({
      ...prev,
      values: { ...prev.values, [key]: checked },
    }))
  }

  function onAddCustom() {
    const side = tab
    const draft = side === 'user' ? userDraft : adminDraft
    const setter = side === 'user' ? setUserDraft : setAdminDraft
    const { draft: next, error } = addCustomKey(draft, side, customInput)
    if (error === 'empty') {
      toast.error(t('Enter a valid menu key (no spaces)'))
      return
    }
    if (error === 'duplicate') {
      toast.error(t('That menu key already exists'))
      return
    }
    setter(next)
    setCustomInput('')
  }

  const isForbidden =
    (query.error as { status?: number } | null)?.status === 403

  function renderSide(side: MenuVisibilitySide, draft: SideDraft) {
    const known = knownKeysFor(side)
    return (
      <div className='bg-card space-y-3 rounded-xl border p-4 shadow-xs'>
        <div className='divide-y'>
          {known.map((def) => (
            <SettingsSwitchRow key={def.key}>
              <SettingsSwitchContent>
                <div className='text-sm font-medium'>{t(def.labelKey)}</div>
                <div className='text-muted-foreground font-mono text-xs'>
                  {def.key}
                </div>
              </SettingsSwitchContent>
              <Switch
                checked={draft.values[def.key] !== false}
                onCheckedChange={(v) => setSideValue(side, def.key, v)}
                disabled={saving}
              />
            </SettingsSwitchRow>
          ))}

          {draft.customKeys.map((key) => (
            <SettingsSwitchRow key={`custom-${key}`}>
              <SettingsSwitchContent>
                <div className='text-sm font-medium'>{key}</div>
                <div className='text-muted-foreground text-xs'>
                  {t('Custom key')}
                </div>
              </SettingsSwitchContent>
              <Switch
                checked={draft.values[key] !== false}
                onCheckedChange={(v) => setSideValue(side, key, v)}
                disabled={saving}
              />
            </SettingsSwitchRow>
          ))}
        </div>

        {tab === side ? (
          <div className='flex flex-wrap items-end gap-2 border-t pt-3'>
            <div className='min-w-48 flex-1 space-y-1'>
              <label className='text-muted-foreground text-xs'>
                {t('Add custom menu key')}
              </label>
              <Input
                value={customInput}
                onChange={(e) => setCustomInput(e.target.value)}
                placeholder={t('e.g. {{example}}', {
                  example: 'future_feature',
                })}
                autoComplete='off'
                disabled={saving}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    onAddCustom()
                  }
                }}
              />
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='h-9'
              disabled={saving || !customInput.trim()}
              onClick={onAddCustom}
            >
              <Plus className='size-3.5' />
              {t('Add')}
            </Button>
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <SettingsSection title={t('Menu visibility')}>
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8'
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
          >
            <RefreshCw className='size-3.5' />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            size='sm'
            className='h-8'
            disabled={!dirty || saving}
            onClick={() => void onSaveClick()}
          >
            <Save className='size-3.5' />
            {saving ? t('Saving...') : t('Save Changes')}
          </Button>
        </div>

        {query.isLoading ? (
          <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
            {t('Loading settings...')}
          </div>
        ) : null}

        {isForbidden ? (
          <ErrorState
            title={t('Insufficient permissions')}
            description={t(
              'Root permission is required to manage menu visibility.'
            )}
          />
        ) : null}

        {query.isError && !query.data && !isForbidden ? (
          <ErrorState
            title={t('Failed to load settings')}
            onRetry={() => void query.refetch()}
          />
        ) : null}

        {query.data ? (
          <Tabs
            value={tab}
            onValueChange={(v) => {
              if (v !== 'user' && v !== 'admin') return
              setTab(v)
              setCustomInput('')
            }}
          >
            <TabsList className='h-9'>
              <TabsTrigger value='user'>{t('User menu')}</TabsTrigger>
              <TabsTrigger value='admin'>{t('Admin menu')}</TabsTrigger>
            </TabsList>
            <TabsContent value='user' className='mt-3'>
              {renderSide('user', userDraft)}
            </TabsContent>
            <TabsContent value='admin' className='mt-3'>
              {renderSide('admin', adminDraft)}
            </TabsContent>
          </Tabs>
        ) : null}
      </div>
    </SettingsSection>
  )
}
