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
import {
  ArrowRightLeft,
  Info,
  Pencil,
  Plus,
  Search,
  Trash2,
  X,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  adminGroupsQueryKey,
  createAdminGroup,
  deleteAdminGroup,
  getAdminGroups,
  groupDisplayQueryKey,
  type AdminGroup,
} from '@/features/adty-groups/api'
import { GroupMigrateDialog } from '@/features/adty-groups/components/group-migrate-dialog'
import { GroupTextDialog } from '@/features/adty-groups/components/group-text-dialog'
import { useGroupDisplay } from '@/features/adty-groups/hooks/use-group-display'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  buildGroupPricingRows,
  groupPricingSignature,
  serializeGroupPricingRows,
  type GroupPricingRow,
} from './group-pricing-rows'

type GroupPricingTableProps = {
  groupRatio: string
  userUsableGroups: string
  onChange: (field: string, value: string) => void
  onShowDetail: (name: string) => void
}

const DEFAULT_GROUP_ID = 'default'

export function GroupPricingTable(props: GroupPricingTableProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const display = useGroupDisplay()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState<AdminGroup | null>(null)
  const [migrating, setMigrating] = useState<AdminGroup | null>(null)
  const [deleting, setDeleting] = useState<AdminGroup | null>(null)

  const adminQuery = useQuery({
    queryKey: adminGroupsQueryKey,
    queryFn: getAdminGroups,
  })
  const adminGroups = useMemo(() => adminQuery.data ?? [], [adminQuery.data])
  const adminById = useMemo(() => {
    const map = new Map<string, AdminGroup>()
    for (const group of adminGroups) map.set(group.id, group)
    return map
  }, [adminGroups])
  const adminIds = useMemo(() => adminGroups.map((g) => g.id), [adminGroups])

  const [rows, setRows] = useState<GroupPricingRow[]>(() =>
    buildGroupPricingRows(props.groupRatio, props.userUsableGroups, adminIds)
  )

  // Rebuild when the saved settings or the group list change, but keep rows
  // that already match them so unsaved edits are not thrown away.
  useEffect(() => {
    const target = buildGroupPricingRows(
      props.groupRatio,
      props.userUsableGroups,
      adminIds
    )
    setRows((current) => {
      const same =
        groupPricingSignature(current, props.userUsableGroups) ===
          groupPricingSignature(target, props.userUsableGroups) &&
        current.length === target.length
      return same ? current : target
    })
  }, [props.groupRatio, props.userUsableGroups, adminIds])

  const { onChange, userUsableGroups } = props
  const emitRows = useCallback(
    (nextRows: GroupPricingRow[]) => {
      setRows(nextRows)
      const serialized = serializeGroupPricingRows(nextRows, userUsableGroups)
      onChange('GroupRatio', serialized.GroupRatio)
      onChange('UserUsableGroups', serialized.UserUsableGroups)
    },
    [onChange, userUsableGroups]
  )

  const updateRow = useCallback(
    (
      id: string,
      field: Exclude<keyof GroupPricingRow, 'id'>,
      value: string | boolean
    ) => {
      emitRows(
        rows.map((row) => (row.id === id ? { ...row, [field]: value } : row))
      )
    },
    [emitRows, rows]
  )

  const createMutation = useMutation({
    mutationFn: () => createAdminGroup({ name: t('New group'), desc: '' }),
    onSuccess: async (created) => {
      setSearch('')
      emitRows([
        { id: created.id, ratio: '1', selectable: true, description: '' },
        ...rows.filter((row) => row.id !== created.id),
      ])
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: adminGroupsQueryKey }),
        queryClient.invalidateQueries({ queryKey: groupDisplayQueryKey }),
      ])
      setEditing(created)
      toast.success(
        t('Group {{id}} created. Save group settings to keep its ratio.', {
          id: created.id,
        })
      )
    },
    onError: (error) => handleServerError(error),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAdminGroup(id),
    onSuccess: async () => {
      setDeleting(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: adminGroupsQueryKey }),
        queryClient.invalidateQueries({ queryKey: groupDisplayQueryKey }),
        queryClient.invalidateQueries({ queryKey: ['system-options'] }),
      ])
      toast.success(t('Group deleted'))
    },
    onError: (error) => handleServerError(error),
  })

  const query = search.trim().toLowerCase()
  const visibleRows = rows.filter(
    (row) =>
      !query ||
      row.id.toLowerCase().includes(query) ||
      display.name(row.id).toLowerCase().includes(query) ||
      row.description.toLowerCase().includes(query)
  )

  const refsText = (id: string) => {
    const refs = adminById.get(id)?.refs
    if (!refs) return '-'
    return t('{{users}} users · {{tokens}} keys · {{channels}} channels', {
      users: formatNumber(refs.users, locale),
      tokens: formatNumber(refs.tokens, locale),
      channels: formatNumber(refs.channels, locale),
    })
  }

  const deleteBlocked = (id: string) => {
    const refs = adminById.get(id)?.refs
    if (!refs || id === DEFAULT_GROUP_ID) return true
    return refs.users + refs.tokens + refs.channels > 0
  }

  return (
    <Card className='min-w-0 shadow-none'>
      <CardHeader className='gap-2 border-b'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div>
            <CardTitle>{t('Pricing groups')}</CardTitle>
            <CardDescription>
              {t(
                'Each group has a fixed ID. Rename it in any language without affecting users or keys; to remove a group, migrate its users, keys and channels first.'
              )}
            </CardDescription>
          </div>
          <Button
            onClick={() => createMutation.mutate()}
            size='sm'
            className='sm:self-start'
            disabled={createMutation.isPending}
          >
            <Plus className='mr-2 h-4 w-4' />
            {t('Add group')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className='flex min-w-0 flex-col gap-4'>
          <InputGroup className='max-w-sm'>
            <InputGroupAddon>
              <Search aria-hidden='true' />
            </InputGroupAddon>
            <InputGroupInput
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              aria-label={t('Search groups by name or description')}
              placeholder={t('Search groups by name or description')}
            />
            {search && (
              <InputGroupAddon align='inline-end'>
                <InputGroupButton
                  size='icon-xs'
                  aria-label={t('Clear search')}
                  onClick={() => setSearch('')}
                >
                  <X aria-hidden='true' />
                </InputGroupButton>
              </InputGroupAddon>
            )}
          </InputGroup>
          <StaticDataTable
            tableClassName='min-w-[860px]'
            tableProps={{ 'aria-label': t('Pricing groups') }}
            data={visibleRows}
            getRowKey={(row) => row.id}
            emptyClassName='text-muted-foreground h-20 text-sm'
            emptyContent={
              <EmptyState
                className='min-h-40'
                title={
                  query
                    ? t('No results found')
                    : t('No groups yet. Add a group to get started.')
                }
                action={
                  query ? (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setSearch('')}
                    >
                      {t('Clear search')}
                    </Button>
                  ) : undefined
                }
              />
            }
            columns={[
              {
                id: 'id',
                header: t('Group ID'),
                className: 'w-28',
                cell: (row) => (
                  <code className='font-mono text-xs' data-testid='group-id'>
                    {row.id}
                  </code>
                ),
              },
              {
                id: 'group',
                header: t('Group name'),
                className: 'min-w-40',
                cell: (row) => {
                  const admin = adminById.get(row.id)
                  return (
                    <Button
                      variant='ghost'
                      size='sm'
                      className='h-auto justify-start gap-2 px-2 font-normal'
                      disabled={!admin}
                      aria-label={t('Edit names of {{group}}', {
                        group: row.id,
                      })}
                      onClick={() => admin && setEditing(admin)}
                    >
                      <span className='truncate'>{display.name(row.id)}</span>
                      <Pencil className='h-3 w-3 shrink-0' aria-hidden='true' />
                    </Button>
                  )
                },
              },
              {
                id: 'ratio',
                header: t('Ratio'),
                className: 'w-28',
                cell: (row) => (
                  <Input
                    type='number'
                    min={0}
                    step={0.0001}
                    value={row.ratio}
                    aria-label={t('Ratio')}
                    onChange={(event) =>
                      updateRow(row.id, 'ratio', event.target.value)
                    }
                  />
                ),
              },
              {
                id: 'selectable',
                header: t('User selectable'),
                className: 'w-28 text-center',
                cell: (row) => (
                  <div className='flex justify-center'>
                    <Checkbox
                      checked={row.selectable}
                      onCheckedChange={(checked) =>
                        updateRow(row.id, 'selectable', checked === true)
                      }
                      aria-label={t('User selectable')}
                    />
                  </div>
                ),
              },
              {
                id: 'description',
                header: t('Description'),
                className: 'min-w-48',
                cell: (row) =>
                  row.selectable ? (
                    <Input
                      value={row.description}
                      aria-label={t('Group description')}
                      placeholder={t('Group description')}
                      onChange={(event) =>
                        updateRow(row.id, 'description', event.target.value)
                      }
                    />
                  ) : (
                    <span className='text-muted-foreground px-3 text-sm'>
                      -
                    </span>
                  ),
              },
              {
                id: 'refs',
                header: t('In use'),
                className: 'min-w-44',
                cell: (row) => (
                  <span
                    className='text-muted-foreground text-xs'
                    data-testid='group-refs'
                  >
                    {refsText(row.id)}
                  </span>
                ),
              },
              {
                id: 'actions',
                header: t('Actions'),
                className: 'text-right',
                cellClassName: 'text-right',
                cell: (row) => {
                  const admin = adminById.get(row.id)
                  return (
                    <div className='flex justify-end gap-1'>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => props.onShowDetail(row.id)}
                        aria-label={t('Details')}
                      >
                        <Info className='h-4 w-4' />
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        disabled={!admin}
                        onClick={() => admin && setMigrating(admin)}
                        aria-label={t('Migrate group')}
                      >
                        <ArrowRightLeft className='h-4 w-4' />
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        disabled={deleteBlocked(row.id)}
                        onClick={() => admin && setDeleting(admin)}
                        aria-label={t('Delete group')}
                        title={
                          deleteBlocked(row.id)
                            ? t('Migrate its users, keys and channels first')
                            : undefined
                        }
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  )
                },
              },
            ]}
          />
        </div>
      </CardContent>
      <GroupTextDialog
        group={editing}
        onOpenChange={(open) => !open && setEditing(null)}
      />
      <GroupMigrateDialog
        group={migrating}
        groups={adminGroups}
        onOpenChange={(open) => !open && setMigrating(null)}
      />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('Delete group')}
        desc={t(
          'Delete {{group}}? It is removed from every group setting. Old logs still show its name.',
          { group: deleting ? display.adminLabel(deleting.id) : '' }
        )}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </Card>
  )
}
