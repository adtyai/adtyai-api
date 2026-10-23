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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsSection } from '../components/settings-section'
import {
  grantHiddenGroup,
  listGroupGrants,
  revokeHiddenGroup,
  saveGroupBilling,
} from './api'
import { CreateGroupDialog } from './components/create-group-dialog'
import { useGroupMetas } from './hooks/use-groups'
import { buildGroupTableSave } from './lib/form'
import type { GroupGrantView } from './types'

type Draft = {
  displayName: string
  ratio: string
  isPublic: boolean
}

function parseObject(raw?: string): Record<string, unknown> | null {
  if (!raw || !raw.trim()) return {}
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return null
    }
    return parsed as Record<string, unknown>
  } catch {
    return null
  }
}

function grantErrorKey(code?: string) {
  switch (code) {
    case 'invalid_code':
      return 'Invalid group code'
    case 'not_hidden':
      return 'Only hidden groups can be granted'
    case 'no_ratio':
      return 'This group has no ratio'
    case 'user_missing':
      return 'User does not exist'
    case 'exists':
      return 'This user already has this group'
    case 'not_found':
      return 'Grant not found'
    case 'invalid_parameters':
    case 'invalid_id':
      return 'Invalid parameters'
    case 'root_required':
      return 'Root permission is required'
    default:
      return 'Save failed'
  }
}

function GroupGrantDialog({
  code,
  open,
  onOpenChange,
}: {
  code: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [userId, setUserId] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const grants = useQuery({
    queryKey: ['group-grants', code],
    queryFn: () => listGroupGrants(code),
    enabled: open && code !== '',
  })

  useEffect(() => {
    if (!open) {
      setUserId('')
      setError('')
    }
  }, [open])

  const items = grants.data?.data ?? []

  async function grant() {
    const id = Number(userId)
    if (!Number.isInteger(id) || id <= 0) {
      setError(t('Invalid parameters'))
      return
    }
    setBusy(true)
    setError('')
    try {
      const res = await grantHiddenGroup(code, id)
      if (!res.success) {
        setError(t(grantErrorKey(res.code)))
        return
      }
      setUserId('')
      await grants.refetch()
    } finally {
      setBusy(false)
    }
  }

  async function revoke(row: GroupGrantView) {
    setBusy(true)
    setError('')
    try {
      const res = await revokeHiddenGroup(code, row.user_id)
      if (!res.success) {
        setError(t(grantErrorKey(res.code)))
        return
      }
      await grants.refetch()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Hidden group grants')}</DialogTitle>
          <DialogDescription>
            {t('Grant this hidden group to one user. Their identity group stays the same.')}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-3'>
          <div className='flex gap-2'>
            <Input
              value={userId}
              inputMode='numeric'
              placeholder={t('User ID')}
              onChange={(event) => setUserId(event.target.value)}
            />
            <Button type='button' disabled={busy} onClick={() => void grant()}>
              {t('Grant')}
            </Button>
          </div>
          {error ? <p className='text-destructive text-sm'>{error}</p> : null}
          {grants.isError || grants.data?.success === false ? (
            <p className='text-destructive text-sm'>{t('Failed to load')}</p>
          ) : items.length === 0 ? (
            <p className='text-muted-foreground text-sm'>{t('No grants')}</p>
          ) : (
            <ul className='divide-y rounded-md border'>
              {items.map((row) => (
                <li
                  key={row.user_id}
                  className='flex items-center justify-between gap-3 px-3 py-2 text-sm'
                >
                  <span>
                    {row.display_name || row.username} ({row.user_id})
                  </span>
                  <Button
                    type='button'
                    variant='outline'
                    disabled={busy}
                    onClick={() => void revoke(row)}
                  >
                    {t('Revoke')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <DialogFooter>
          <Button type='button' variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function GroupSimpleSection({
  groupRatio,
  userUsableGroups,
}: {
  groupRatio?: string
  userUsableGroups?: string
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const metas = useGroupMetas()
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [saving, setSaving] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [grantCode, setGrantCode] = useState('')

  const parsed = useMemo(() => {
    const ratio = parseObject(groupRatio)
    const usable = parseObject(userUsableGroups)
    return { ratio, usable }
  }, [groupRatio, userUsableGroups])

  const codes = useMemo(() => {
    if (!parsed.ratio) return []
    return Object.keys(parsed.ratio).sort((a, b) => a.localeCompare(b))
  }, [parsed.ratio])

  useEffect(() => {
    if (!parsed.ratio || !parsed.usable) return
    const next: Record<string, Draft> = {}
    for (const code of codes) {
      const meta = metas.data?.find((item) => item.code === code)
      const ratioValue = parsed.ratio[code]
      next[code] = {
        displayName:
          meta?.display_name ||
          (typeof parsed.usable[code] === 'string' ? parsed.usable[code] : code),
        ratio:
          typeof ratioValue === 'number' && Number.isFinite(ratioValue)
            ? String(ratioValue)
            : '',
        isPublic: Object.prototype.hasOwnProperty.call(parsed.usable, code),
      }
    }
    setDrafts(next)
  }, [codes, metas.data, parsed.ratio, parsed.usable])

  function patch(code: string, patchValue: Partial<Draft>) {
    setDrafts((current) => ({
      ...current,
      [code]: { ...current[code], ...patchValue },
    }))
  }

  async function refreshOptions() {
    await qc.invalidateQueries({ queryKey: ['system-options'] })
    await qc.invalidateQueries({ queryKey: ['status'] })
    await qc.invalidateQueries({ queryKey: ['user-groups'] })
    try {
      window.localStorage.removeItem('status')
    } catch {
      /* Storage may be disabled. */
    }
  }

  async function save() {
    if (!parsed.ratio || !parsed.usable || !isRoot) return
    const currentDisplayNames: Record<string, string> = {}
    for (const meta of metas.data ?? []) {
      currentDisplayNames[meta.code] = meta.display_name || ''
    }
    const built = buildGroupTableSave({
      ratio: parsed.ratio,
      usable: parsed.usable,
      drafts,
      currentDisplayNames,
    })
    if (!built.ok) {
      toast.error(
        t(
          built.error === 'draft_missing'
            ? 'Group settings could not be read. Nothing was saved.'
            : 'Ratio must be a number'
        )
      )
      return
    }
    if (!built.changed) {
      toast.success(t('Saved'))
      return
    }
    setSaving(true)
    try {
      const res = await saveGroupBilling({
        group_ratio: built.ratio,
        user_usable_groups: built.usable,
        display_names: built.displayNames,
      })
      if (!res.success) {
        toast.error(res.message || t('Save failed'))
        return
      }
      await refreshOptions()
      toast.success(t('Saved'))
    } catch {
      toast.error(t('Save failed'))
    } finally {
      setSaving(false)
    }
  }

  async function onCreated(_code: string) {
    await refreshOptions()
  }

  const invalid = !parsed.ratio || !parsed.usable

  return (
    <SettingsSection title={t('Group Pricing')}>
      <div className='space-y-3'>
        <p className='text-muted-foreground text-sm'>
          {t('Group code cannot be changed')}
          {' '}
          {t('A hidden group is granted to one user and does not replace that user identity group.')}
        </p>
        {invalid ? (
          <p className='text-destructive text-sm'>
            {t('Group settings could not be read. Nothing was saved.')}
          </p>
        ) : null}
        <div className='overflow-x-auto rounded-md border'>
          <table className='w-full text-sm'>
            <thead className='bg-muted/40 text-left'>
              <tr>
                <th className='px-3 py-2 font-medium'>{t('Group')}</th>
                <th className='px-3 py-2 font-medium'>{t('Display name')}</th>
                <th className='px-3 py-2 font-medium'>{t('Ratio')}</th>
                <th className='px-3 py-2 font-medium'>{t('Visible to everyone')}</th>
                <th className='px-3 py-2 font-medium'>{t('Grant')}</th>
              </tr>
            </thead>
            <tbody>
              {codes.map((code) => {
                const draft = drafts[code]
                const locked = code === 'auto' || !isRoot
                return (
                  <tr key={code} className='border-t'>
                    <td className='px-3 py-2 font-mono'>{code}</td>
                    <td className='px-3 py-2'>
                      <Input
                        value={draft?.displayName ?? ''}
                        disabled={locked || saving}
                        onChange={(event) =>
                          patch(code, { displayName: event.target.value })
                        }
                      />
                      {code === 'auto' ? (
                        <p className='text-muted-foreground mt-1 text-xs'>
                          {t('Auto group is no longer available')}
                        </p>
                      ) : null}
                    </td>
                    <td className='px-3 py-2'>
                      <Input
                        value={draft?.ratio ?? ''}
                        disabled={locked || saving}
                        onChange={(event) => patch(code, { ratio: event.target.value })}
                      />
                    </td>
                    <td className='px-3 py-2'>
                      <Checkbox
                        checked={draft?.isPublic ?? false}
                        disabled={locked || saving}
                        onCheckedChange={(checked) =>
                          patch(code, { isPublic: checked === true })
                        }
                      />
                    </td>
                    <td className='px-3 py-2'>
                      {code !== 'auto' && !draft?.isPublic ? (
                        <Button
                          type='button'
                          variant='outline'
                          disabled={!isRoot}
                          onClick={() => setGrantCode(code)}
                        >
                          {t('Grant')}
                        </Button>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className='flex gap-2'>
          <Button type='button' disabled={!isRoot || saving || invalid} onClick={() => void save()}>
            {t('Save')}
          </Button>
          <Button
            type='button'
            variant='outline'
            disabled={!isRoot || saving}
            onClick={() => setCreateOpen(true)}
          >
            {t('New group')}
          </Button>
        </div>
      </div>
      <CreateGroupDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(code) => void onCreated(code)}
      />
      <GroupGrantDialog
        code={grantCode}
        open={grantCode !== ''}
        onOpenChange={(open) => {
          if (!open) setGrantCode('')
        }}
      />
    </SettingsSection>
  )
}
