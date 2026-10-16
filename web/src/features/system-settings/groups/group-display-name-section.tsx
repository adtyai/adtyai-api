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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ROLE } from '@/lib/roles'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsSection } from '../components/settings-section'
import { CreateGroupDialog } from './components/create-group-dialog'
import { useGroupMetas, usePutGroupMeta } from './hooks/use-groups'
import {
  buildGroupPut,
  groupApiErrorToastKey,
  viewToDraft,
} from './lib/form'
import type { GroupMetaView, GroupRowDraft } from './types'

/**
 * Embeddable group display-name block for Group Pricing billing section.
 * Root/SUPER_ADMIN only; non-root sees nothing (sibling pricing stays usable).
 */
export function GroupDisplayNameSection() {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const list = useGroupMetas({ enabled: isRoot })
  const putMutation = usePutGroupMeta()
  const [createOpen, setCreateOpen] = useState(false)
  const [drafts, setDrafts] = useState<Record<string, GroupRowDraft>>({})
  const [savingCode, setSavingCode] = useState<string | null>(null)

  useEffect(() => {
    if (!list.data) return
    const next: Record<string, GroupRowDraft> = {}
    for (const row of list.data) {
      next[row.code] = viewToDraft(row)
    }
    setDrafts(next)
  }, [list.data])

  const isForbidden =
    (list.error as { status?: number } | null)?.status === 403

  const sortedRows = useMemo(() => {
    const rows = list.data ?? []
    return [...rows].sort((a, b) => {
      if (a.sort !== b.sort) return a.sort - b.sort
      return a.code.localeCompare(b.code)
    })
  }, [list.data])

  function setDraftField(
    code: string,
    field: keyof GroupRowDraft,
    value: string
  ) {
    setDrafts((prev) => ({
      ...prev,
      [code]: {
        ...(prev[code] ?? { display_name: '', sort: '0' }),
        [field]: value,
      },
    }))
  }

  function isRowDirty(row: GroupMetaView): boolean {
    const draft = drafts[row.code]
    if (!draft) return false
    const initial = viewToDraft(row)
    return (
      draft.display_name !== initial.display_name ||
      draft.sort !== initial.sort
    )
  }

  async function onSaveRow(row: GroupMetaView) {
    if (!isRoot) {
      toast.error(
        t('Root permission is required to manage group display names.')
      )
      return
    }
    const draft = drafts[row.code] ?? viewToDraft(row)
    const built = buildGroupPut(row.code, draft, row)
    if ('error' in built) {
      if (built.error === 'display_name_required') {
        toast.error(t('Display name is required'))
        return
      }
      if (built.error === 'invalid_sort') {
        toast.error(t('Sort must be an integer'))
        return
      }
      toast.info(t('No changes to save'))
      return
    }

    setSavingCode(row.code)
    try {
      const res = await putMutation.mutateAsync({
        code: row.code,
        input: built.body,
      })
      if (!res.success) {
        if (res.code === 'forbidden' || res.code === 'root_required') {
          toast.error(
            t('Root permission is required to manage group display names.')
          )
          return
        }
        const key = groupApiErrorToastKey(res.code)
        toast.error(
          key ? t(key) : getServerErrorMessage(res, t('Save failed'))
        )
        return
      }
      toast.success(t('Saved'))
      void list.refetch()
    } catch (err) {
      const status = (err as { status?: number })?.status
      const code = (err as { code?: string } | null)?.code
      if (status === 403 || code === 'root_required') {
        toast.error(
          t('Root permission is required to manage group display names.')
        )
        return
      }
      toast.error(getServerErrorMessage(err, t('Save failed')))
    } finally {
      setSavingCode(null)
    }
  }

  // Hooks above must run unconditionally; gate render after.
  if (!isRoot) {
    return null
  }

  return (
    <SettingsSection title={t('Group display names')}>
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8'
            onClick={() => void list.refetch()}
            disabled={list.isFetching}
          >
            <RefreshCw className='size-3.5' />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            size='sm'
            className='h-8'
            onClick={() => setCreateOpen(true)}
          >
            <Plus className='size-3.5' />
            {t('Create')}
          </Button>
        </div>

        {list.isLoading ? (
          <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
            {t('Loading settings...')}
          </div>
        ) : null}

        {isForbidden ? (
          <ErrorState
            title={t('Insufficient permissions')}
            description={t(
              'Root permission is required to manage group display names.'
            )}
          />
        ) : null}

        {list.isError && !list.data && !isForbidden ? (
          <ErrorState
            title={t('Failed to load settings')}
            onRetry={() => void list.refetch()}
          />
        ) : null}

        {list.data && !isForbidden ? (
          <div className='bg-card overflow-hidden rounded-xl border shadow-xs'>
            <Table>
              <TableHeader>
                <TableRow className='bg-muted/40 hover:bg-muted/40'>
                  <TableHead className='w-[20%]'>{t('Group code')}</TableHead>
                  <TableHead className='w-[35%]'>{t('Display name')}</TableHead>
                  <TableHead className='w-[12%]'>{t('Sort')}</TableHead>
                  <TableHead className='w-[15%]'>{t('Meta')}</TableHead>
                  <TableHead className='w-[1%]' />
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedRows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className='text-muted-foreground h-24 text-center text-sm'
                    >
                      {t('No groups yet. Create one or wait for in-use codes to appear.')}
                    </TableCell>
                  </TableRow>
                ) : (
                  sortedRows.map((row) => {
                    const draft = drafts[row.code] ?? viewToDraft(row)
                    const dirty = isRowDirty(row)
                    const busy = savingCode === row.code
                    return (
                      <TableRow key={row.code}>
                        <TableCell>
                          <code className='font-mono text-xs'>{row.code}</code>
                        </TableCell>
                        <TableCell>
                          <Input
                            value={draft.display_name}
                            onChange={(e) =>
                              setDraftField(
                                row.code,
                                'display_name',
                                e.target.value
                              )
                            }
                            disabled={busy || putMutation.isPending}
                            className='h-8'
                            autoComplete='off'
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            value={draft.sort}
                            onChange={(e) =>
                              setDraftField(row.code, 'sort', e.target.value)
                            }
                            disabled={busy || putMutation.isPending}
                            className='h-8 w-24 font-mono'
                            inputMode='numeric'
                            autoComplete='off'
                          />
                        </TableCell>
                        <TableCell>
                          {row.has_meta ? (
                            <Badge variant='secondary'>{t('Saved meta')}</Badge>
                          ) : (
                            <Badge
                              variant='outline'
                              className='text-muted-foreground'
                            >
                              {t('In use (no meta)')}
                            </Badge>
                          )}
                        </TableCell>
                        <TableCell>
                          <Button
                            type='button'
                            size='sm'
                            className='h-8'
                            disabled={!dirty || busy || putMutation.isPending}
                            onClick={() => void onSaveRow(row)}
                          >
                            <Save className='size-3.5' />
                            {busy ? t('Saving...') : t('Save')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
        ) : null}
      </div>

      <CreateGroupDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => void list.refetch()}
      />
    </SettingsSection>
  )
}
