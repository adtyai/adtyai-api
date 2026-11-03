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
import { AlertTriangle, ChevronDown, Plus, Trash2 } from 'lucide-react'
import { Reorder } from 'motion/react'
import {
  useState,
  useMemo,
  useEffect,
  useCallback,
  memo,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Dialog } from '@/components/dialog'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useGroupDisplay } from '@/features/adty-groups/hooks/use-group-display'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { safeJsonParse } from '../utils/json-parser'
import { GroupPricingTable } from './group-pricing-table'
import { GroupSpecialUsableRulesEditor } from './group-special-usable-editor'

export type GroupSettingsSection =
  | 'pricing'
  | 'overrides'
  | 'visibility'
  | 'auto'

type GroupRatioVisualEditorProps = {
  section: GroupSettingsSection
  onSectionChange: (section: GroupSettingsSection) => void
  defaultUseAutoGroupField: ReactNode
  groupRatio: string
  topupGroupRatio: string
  userUsableGroups: string
  groupGroupRatio: string
  autoGroups: string
  maxTokenAutoGroupsField: ReactNode
  groupSpecialUsableGroup: string
  onChange: (field: string, value: string) => void
}

type RegistryEntry = {
  name: string
  ratio: number
}

const sectionCardClassName = 'min-w-0 shadow-none'
const sectionHeaderClassName = 'gap-2 border-b'

function normalizeRatio(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 1
}

function parseRatioMap(value: string): Record<string, number> {
  return safeJsonParse<Record<string, number>>(value, {
    fallback: {},
    silent: true,
  })
}

function parseUsableMap(value: string): Record<string, string> {
  return safeJsonParse<Record<string, string>>(value, {
    fallback: {},
    silent: true,
  })
}

function parseNestedRatioMap(
  value: string
): Record<string, Record<string, number>> {
  return safeJsonParse<Record<string, Record<string, number>>>(value, {
    fallback: {},
    silent: true,
  })
}

function UnknownGroupBadge() {
  const { t } = useTranslation()
  return (
    <StatusBadge variant='danger' copyable={false}>
      <AlertTriangle className='mr-1 h-3 w-3' />
      {t('Not in pricing table')}
    </StatusBadge>
  )
}

type GroupNameSelectProps = {
  options: string[]
  value: string | null
  placeholder: string
  onValueChange: (value: string) => void
  className?: string
}

function GroupNameSelect(props: GroupNameSelectProps) {
  const display = useGroupDisplay()
  const options = useMemo(() => {
    if (props.value && !props.options.includes(props.value)) {
      return [props.value, ...props.options]
    }
    return props.options
  }, [props.options, props.value])

  return (
    <Combobox
      options={options.map((name) => ({
        value: name,
        label: display.adminLabel(name),
      }))}
      value={props.value}
      onValueChange={(value) => {
        if (value) props.onValueChange(value)
      }}
      className={props.className ?? 'w-48'}
      placeholder={props.placeholder}
      aria-label={props.placeholder}
    />
  )
}

export const GroupRatioVisualEditor = memo(function GroupRatioVisualEditor({
  section,
  onSectionChange,
  defaultUseAutoGroupField,
  groupRatio,
  topupGroupRatio,
  userUsableGroups,
  groupGroupRatio,
  autoGroups,
  maxTokenAutoGroupsField,
  groupSpecialUsableGroup,
  onChange,
}: GroupRatioVisualEditorProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const display = useGroupDisplay()
  const [detailGroup, setDetailGroup] = useState<string | null>(null)

  const registry = useMemo<RegistryEntry[]>(() => {
    const ratioMap = parseRatioMap(groupRatio)
    const usableMap = parseUsableMap(userUsableGroups)
    const topupMap = parseRatioMap(topupGroupRatio)
    const names = new Set([
      ...Object.keys(ratioMap),
      ...Object.keys(usableMap),
      ...Object.keys(topupMap),
    ])
    return [...names].map((name) => ({
      name,
      ratio: normalizeRatio(ratioMap[name]),
    }))
  }, [groupRatio, userUsableGroups, topupGroupRatio])

  const registryNames = useMemo(
    () => registry.map((entry) => entry.name),
    [registry]
  )

  // Auto groups
  const autoGroupsList = useMemo(() => {
    return safeJsonParse<string[]>(autoGroups, {
      fallback: [],
      context: 'auto groups',
    })
  }, [autoGroups])

  const handleAutoGroupAdd = useCallback(
    (name: string) => {
      if (autoGroupsList.includes(name)) return
      onChange('AutoGroups', JSON.stringify([...autoGroupsList, name], null, 2))
    },
    [autoGroupsList, onChange]
  )

  const handleAutoGroupDelete = useCallback(
    (index: number) => {
      const list = autoGroupsList.filter((_, i) => i !== index)
      onChange('AutoGroups', JSON.stringify(list, null, 2))
    },
    [autoGroupsList, onChange]
  )

  const handleAutoGroupMove = useCallback(
    (index: number, direction: 'up' | 'down') => {
      const list = [...autoGroupsList]
      const newIndex = direction === 'up' ? index - 1 : index + 1
      if (newIndex < 0 || newIndex >= list.length) return
      ;[list[index], list[newIndex]] = [list[newIndex], list[index]]
      onChange('AutoGroups', JSON.stringify(list, null, 2))
    },
    [autoGroupsList, onChange]
  )

  const autoGroupCandidates = useMemo(
    () => registryNames.filter((name) => !autoGroupsList.includes(name)),
    [registryNames, autoGroupsList]
  )

  return (
    <Tabs
      value={section}
      onValueChange={(value) => onSectionChange(value as GroupSettingsSection)}
      className='min-w-0 gap-5'
    >
      <div className='min-w-0 overflow-x-auto pb-1'>
        <TabsList aria-label={t('Group settings')} className='w-full min-w-max'>
          <TabsTrigger value='pricing' className='px-3'>
            {t('Pricing groups')}
          </TabsTrigger>
          <TabsTrigger value='overrides' className='px-3'>
            {t('Special ratio rules')}
          </TabsTrigger>
          <TabsTrigger value='visibility' className='px-3'>
            {t('Group visibility')}
          </TabsTrigger>
          <TabsTrigger value='auto' className='px-3'>
            {t('Auto group order')}
          </TabsTrigger>
        </TabsList>
      </div>
      <TabsContent value='pricing' keepMounted>
        <GroupPricingTable
          groupRatio={groupRatio}
          userUsableGroups={userUsableGroups}
          onChange={onChange}
          onShowDetail={setDetailGroup}
        />
      </TabsContent>
      <TabsContent value='overrides' keepMounted>
        <GroupOverrideRules
          registry={registry}
          groupGroupRatio={groupGroupRatio}
          onChange={onChange}
        />
      </TabsContent>
      <TabsContent value='visibility' keepMounted>
        <GroupSpecialUsableRulesEditor
          value={groupSpecialUsableGroup}
          groupOptions={registryNames}
          onChange={(value) => onChange('GroupSpecialUsableGroup', value)}
        />
      </TabsContent>
      <TabsContent value='auto' keepMounted>
        <Card className={sectionCardClassName}>
          <CardHeader className={sectionHeaderClassName}>
            <CardTitle>{t('Auto group order')}</CardTitle>
            <CardDescription>
              {t(
                'Priority order for tokens in the auto group. The system tries groups from top to bottom.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(16rem,0.7fr)]'>
            <div className='flex min-w-0 flex-col gap-3'>
              <GroupNameSelect
                options={autoGroupCandidates}
                value={null}
                className='w-full'
                placeholder={t('Add group')}
                onValueChange={handleAutoGroupAdd}
              />
              {autoGroupsList.length === 0 ? (
                <EmptyState
                  className='min-h-40'
                  title={t('No auto groups configured')}
                  description={t(
                    'Add groups in the order they should be tried.'
                  )}
                />
              ) : (
                <Reorder.Group
                  as='ol'
                  axis='y'
                  values={autoGroupsList}
                  onReorder={(groups) =>
                    onChange('AutoGroups', JSON.stringify(groups, null, 2))
                  }
                  aria-label={t('Auto group order')}
                  className='flex flex-col gap-2'
                >
                  {autoGroupsList.map((group, index) => (
                    <AutoGroupOrderItem
                      key={group}
                      group={group}
                      label={display.adminLabel(group)}
                      index={index}
                      count={autoGroupsList.length}
                      onMove={handleAutoGroupMove}
                      onRemove={() => handleAutoGroupDelete(index)}
                      leading={
                        <span className='text-muted-foreground w-5 shrink-0 text-center text-sm tabular-nums'>
                          {formatNumber(index + 1, locale)}
                        </span>
                      }
                    >
                      {!registryNames.includes(group) && <UnknownGroupBadge />}
                    </AutoGroupOrderItem>
                  ))}
                </Reorder.Group>
              )}
            </div>
            <div className='bg-muted/20 flex min-w-0 flex-col gap-4 self-start rounded-lg border p-4'>
              {defaultUseAutoGroupField}
              <Separator />
              {maxTokenAutoGroupsField}
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <GroupDetailSheet
        groupName={detailGroup}
        onOpenChange={(open) => {
          if (!open) setDetailGroup(null)
        }}
        registry={registry}
        topupGroupRatio={topupGroupRatio}
        userUsableGroups={userUsableGroups}
        groupGroupRatio={groupGroupRatio}
        autoGroups={autoGroupsList}
        groupSpecialUsableGroup={groupSpecialUsableGroup}
      />
    </Tabs>
  )
})

type GroupOverride = {
  targetGroup: string
  ratio: number
}

type GroupOverrideRulesProps = {
  registry: RegistryEntry[]
  groupGroupRatio: string
  onChange: (field: string, value: string) => void
}

function GroupOverrideRules({
  registry,
  groupGroupRatio,
  onChange,
}: GroupOverrideRulesProps) {
  const { t } = useTranslation()
  const display = useGroupDisplay()
  const [userGroupDialogOpen, setUserGroupDialogOpen] = useState(false)
  const [userGroupInput, setUserGroupInput] = useState<string | null>(null)
  const [overrideDialogOpen, setOverrideDialogOpen] = useState(false)
  const [overrideUserGroup, setOverrideUserGroup] = useState<string | null>(
    null
  )
  const [overrideEditData, setOverrideEditData] =
    useState<GroupOverride | null>(null)

  const registryNames = useMemo(
    () => registry.map((entry) => entry.name),
    [registry]
  )

  const baseRatioByName = useMemo(() => {
    const map = new Map<string, number>()
    for (const entry of registry) map.set(entry.name, entry.ratio)
    return map
  }, [registry])

  const groupGroupRatioList = useMemo(() => {
    const map = parseNestedRatioMap(groupGroupRatio)
    return Object.entries(map).map(([userGroup, overrides]) => ({
      userGroup,
      overrides: Object.entries(overrides).map(([targetGroup, ratio]) => ({
        targetGroup,
        ratio,
      })),
    }))
  }, [groupGroupRatio])

  const emitMap = useCallback(
    (map: Record<string, Record<string, number>>) => {
      onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
    },
    [onChange]
  )

  const handleUserGroupSave = useCallback(() => {
    if (!userGroupInput) return
    const map = parseNestedRatioMap(groupGroupRatio)
    if (!map[userGroupInput]) {
      map[userGroupInput] = {}
    }
    emitMap(map)
    setUserGroupDialogOpen(false)
    setUserGroupInput(null)
  }, [userGroupInput, groupGroupRatio, emitMap])

  const handleUserGroupDelete = useCallback(
    (userGroup: string) => {
      const map = parseNestedRatioMap(groupGroupRatio)
      delete map[userGroup]
      emitMap(map)
    },
    [groupGroupRatio, emitMap]
  )

  const handleOverrideAdd = useCallback((userGroup: string) => {
    setOverrideUserGroup(userGroup)
    setOverrideEditData(null)
    setOverrideDialogOpen(true)
  }, [])

  const handleOverrideEdit = useCallback(
    (userGroup: string, override: GroupOverride) => {
      setOverrideUserGroup(userGroup)
      setOverrideEditData(override)
      setOverrideDialogOpen(true)
    },
    []
  )

  const handleOverrideSave = useCallback(
    (targetGroup: string, ratio: number, oldTargetGroup?: string) => {
      if (!overrideUserGroup) return
      const map = parseNestedRatioMap(groupGroupRatio)
      if (!map[overrideUserGroup]) {
        map[overrideUserGroup] = {}
      }
      if (oldTargetGroup && oldTargetGroup !== targetGroup) {
        delete map[overrideUserGroup][oldTargetGroup]
      }
      map[overrideUserGroup][targetGroup] = ratio
      emitMap(map)
      setOverrideDialogOpen(false)
    },
    [overrideUserGroup, groupGroupRatio, emitMap]
  )

  const handleOverrideDelete = useCallback(
    (userGroup: string, targetGroup: string) => {
      const map = parseNestedRatioMap(groupGroupRatio)
      if (map[userGroup]) {
        delete map[userGroup][targetGroup]
        if (Object.keys(map[userGroup]).length === 0) {
          delete map[userGroup]
        }
      }
      emitMap(map)
    },
    [groupGroupRatio, emitMap]
  )

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <CardTitle>{t('Special ratio rules')}</CardTitle>
        <CardDescription>
          {t(
            'Each rule reads as a sentence: users of one group pay a special ratio when billed as another group. Without a rule, the billing group base ratio applies.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='space-y-4'>
          <Button
            onClick={() => {
              setUserGroupInput(null)
              setUserGroupDialogOpen(true)
            }}
            size='sm'
          >
            <Plus className='mr-2 h-4 w-4' />
            {t('Add user group')}
          </Button>
          {groupGroupRatioList.length === 0 && (
            <EmptyState
              className='min-h-40'
              title={t('No special ratios configured')}
              description={t(
                'Base group ratios apply until you add an override.'
              )}
            />
          )}
          {groupGroupRatioList.length > 0 && (
            <div className='space-y-3'>
              {groupGroupRatioList.map((userGroupData) => (
                <Collapsible key={userGroupData.userGroup} defaultOpen>
                  <div className='rounded-lg border'>
                    <div className='flex items-center justify-between gap-2 p-3'>
                      <CollapsibleTrigger
                        render={
                          <Button
                            variant='ghost'
                            className='h-auto min-w-0 justify-start whitespace-normal'
                          />
                        }
                        aria-label={t('Rules for {{group}}', {
                          group: userGroupData.userGroup,
                        })}
                      >
                        <ChevronDown
                          className='size-4 shrink-0'
                          aria-hidden='true'
                        />
                        <span className='min-w-0 truncate font-semibold'>
                          {display.adminLabel(userGroupData.userGroup)}
                        </span>
                        {!registryNames.includes(userGroupData.userGroup) && (
                          <AlertTriangle
                            className='text-destructive h-4 w-4'
                            aria-label={t('Not in pricing table')}
                          />
                        )}
                        <span className='text-muted-foreground text-sm'>
                          {t('{{count}} override', {
                            count: userGroupData.overrides.length,
                          })}
                        </span>
                      </CollapsibleTrigger>
                      <div className='flex shrink-0 gap-1'>
                        <Button
                          variant='ghost'
                          size='sm'
                          aria-label={t('Add ratio override')}
                          onClick={() =>
                            handleOverrideAdd(userGroupData.userGroup)
                          }
                        >
                          <Plus className='h-4 w-4' />
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          aria-label={t('Remove {{group}}', {
                            group: userGroupData.userGroup,
                          })}
                          onClick={() =>
                            handleUserGroupDelete(userGroupData.userGroup)
                          }
                        >
                          <Trash2 className='h-4 w-4' />
                        </Button>
                      </div>
                    </div>
                    <CollapsibleContent>
                      {userGroupData.overrides.length > 0 && (
                        <div className='border-t'>
                          <StaticDataTable
                            className='rounded-none border-0'
                            data={userGroupData.overrides}
                            getRowKey={(override) => override.targetGroup}
                            columns={[
                              {
                                id: 'target-group',
                                header: t('Billing group'),
                                cellClassName: 'font-medium',
                                cell: (override) => (
                                  <span className='inline-flex max-w-48 items-center gap-1.5'>
                                    <span
                                      className='truncate'
                                      title={override.targetGroup}
                                    >
                                      {display.adminLabel(override.targetGroup)}
                                    </span>
                                    {!registryNames.includes(
                                      override.targetGroup
                                    ) && (
                                      <AlertTriangle
                                        className='text-destructive h-3.5 w-3.5'
                                        aria-label={t('Not in pricing table')}
                                      />
                                    )}
                                  </span>
                                ),
                              },
                              {
                                id: 'ratio',
                                header: t('Ratio'),
                                cell: (override) => {
                                  const baseRatio = baseRatioByName.get(
                                    override.targetGroup
                                  )
                                  return (
                                    <span className='inline-flex items-center gap-1.5'>
                                      {override.ratio}
                                      {baseRatio !== undefined &&
                                        baseRatio !== override.ratio && (
                                          <span className='text-muted-foreground text-xs'>
                                            {t('(instead of {{ratio}})', {
                                              ratio: baseRatio,
                                            })}
                                          </span>
                                        )}
                                    </span>
                                  )
                                },
                              },
                              {
                                id: 'actions',
                                header: t('Actions'),
                                className: 'text-right',
                                cellClassName: 'text-right',
                                cell: (override) => (
                                  <StaticRowActions
                                    editLabel={t('Edit')}
                                    deleteLabel={t('Delete')}
                                    menuLabel={t('Open menu')}
                                    onEdit={() =>
                                      handleOverrideEdit(
                                        userGroupData.userGroup,
                                        override
                                      )
                                    }
                                    onDelete={() =>
                                      handleOverrideDelete(
                                        userGroupData.userGroup,
                                        override.targetGroup
                                      )
                                    }
                                  />
                                ),
                              },
                            ]}
                          />
                        </div>
                      )}
                    </CollapsibleContent>
                  </div>
                </Collapsible>
              ))}
            </div>
          )}
        </div>
      </CardContent>

      {/* Add user group dialog */}
      <Dialog
        open={userGroupDialogOpen}
        onOpenChange={setUserGroupDialogOpen}
        title={t('Add user group')}
        description={t(
          'Create a new user group to configure ratio overrides for.'
        )}
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setUserGroupDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleUserGroupSave} disabled={!userGroupInput}>
              {t('Add')}
            </Button>
          </>
        }
      >
        <div className='space-y-4 py-4'>
          <div className='space-y-2'>
            <Label>{t('User group name')}</Label>
            <GroupNameSelect
              className='w-full'
              options={registryNames}
              value={userGroupInput}
              placeholder={t('Select a group')}
              onValueChange={setUserGroupInput}
            />
          </div>
        </div>
      </Dialog>

      <GroupOverrideDialog
        open={overrideDialogOpen}
        onOpenChange={setOverrideDialogOpen}
        onSave={handleOverrideSave}
        editData={overrideEditData}
        userGroup={overrideUserGroup}
        groupOptions={registryNames}
        baseRatioByName={baseRatioByName}
      />
    </Card>
  )
}

type GroupOverrideDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (targetGroup: string, ratio: number, oldTargetGroup?: string) => void
  editData: GroupOverride | null
  userGroup: string | null
  groupOptions: string[]
  baseRatioByName: Map<string, number>
}

function GroupOverrideDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  userGroup,
  groupOptions,
  baseRatioByName,
}: GroupOverrideDialogProps) {
  const { t } = useTranslation()
  const [targetGroup, setTargetGroup] = useState<string | null>(null)
  const [ratio, setRatio] = useState('')

  useEffect(() => {
    if (!open) {
      setTargetGroup(null)
      setRatio('')
      return
    }

    setTargetGroup(editData?.targetGroup ?? null)
    setRatio(editData ? String(editData.ratio) : '')
  }, [editData, open])

  const baseRatio = targetGroup ? baseRatioByName.get(targetGroup) : undefined

  const handleSave = () => {
    if (!targetGroup || !ratio.trim()) return
    const parsedRatio = Number.parseFloat(ratio)
    if (Number.isNaN(parsedRatio)) return

    onSave(targetGroup, parsedRatio, editData?.targetGroup)
    setTargetGroup(null)
    setRatio('')
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={editData ? t('Edit ratio override') : t('Add ratio override')}
      description={
        userGroup
          ? t(
              'Configure a custom ratio for "{{userGroup}}" users when using a specific token group.',
              { userGroup }
            )
          : t(
              'Configure a custom ratio for when users use a specific token group.'
            )
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave}>
            {editData ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-4'>
        <div className='space-y-2'>
          <Label>{t('Billing group')}</Label>
          <GroupNameSelect
            className='w-full'
            options={groupOptions}
            value={targetGroup}
            placeholder={t('Select a group')}
            onValueChange={setTargetGroup}
          />
          <p className='text-muted-foreground text-xs'>
            {t('The token group that will have a custom ratio')}
          </p>
        </div>
        <div className='space-y-2'>
          <Label>{t('Ratio')}</Label>
          <Input
            value={ratio}
            onChange={(e) => {
              const val = e.target.value
              if (val === '' || !Number.isNaN(Number.parseFloat(val))) {
                setRatio(val)
              }
            }}
            placeholder={baseRatio === undefined ? '0.9' : String(baseRatio)}
          />
          <p className='text-muted-foreground text-xs'>
            {baseRatio !== undefined
              ? t('(instead of {{ratio}})', { ratio: baseRatio })
              : t(
                  'Multiplier applied when {{userGroup}} uses {{targetGroup}}',
                  {
                    userGroup: userGroup || t('this user group'),
                    targetGroup: targetGroup || t('this token group'),
                  }
                )}
          </p>
        </div>
      </div>
    </Dialog>
  )
}

type GroupDetailSheetProps = {
  groupName: string | null
  onOpenChange: (open: boolean) => void
  registry: RegistryEntry[]
  topupGroupRatio: string
  userUsableGroups: string
  groupGroupRatio: string
  autoGroups: string[]
  groupSpecialUsableGroup: string
}

type VisibilityRule = {
  userGroup: string
  visible: boolean
  description: string
}

function parseSpecialGroupKey(rawKey: string): {
  visible: boolean
  groupName: string
} {
  if (rawKey.startsWith('-:')) {
    return { visible: false, groupName: rawKey.slice(2) }
  }
  if (rawKey.startsWith('+:')) {
    return { visible: true, groupName: rawKey.slice(2) }
  }
  return { visible: true, groupName: rawKey }
}

function GroupDetailSheet(props: GroupDetailSheetProps) {
  const { t } = useTranslation()
  const display = useGroupDisplay()
  const name = props.groupName

  const detail = useMemo(() => {
    if (!name) return null

    const entry = props.registry.find((item) => item.name === name)
    const topupMap = parseRatioMap(props.topupGroupRatio)
    const usableMap = parseUsableMap(props.userUsableGroups)
    const overrideMap = parseNestedRatioMap(props.groupGroupRatio)
    const specialMap = safeJsonParse<Record<string, Record<string, string>>>(
      props.groupSpecialUsableGroup,
      { fallback: {}, silent: true }
    )

    // Overrides that apply when other user groups bill as this group
    const incomingOverrides: { userGroup: string; ratio: number }[] = []
    for (const [userGroup, overrides] of Object.entries(overrideMap)) {
      if (Object.hasOwn(overrides, name)) {
        incomingOverrides.push({ userGroup, ratio: overrides[name] })
      }
    }

    // Overrides that apply when users of this group bill as other groups
    const outgoingOverrides = Object.entries(overrideMap[name] ?? {}).map(
      ([targetGroup, ratio]) => ({ targetGroup, ratio })
    )

    // Visibility rules targeting this group
    const visibilityRules: VisibilityRule[] = []
    for (const [userGroup, inner] of Object.entries(specialMap)) {
      if (typeof inner !== 'object' || inner === null) continue
      for (const [rawKey, desc] of Object.entries(inner)) {
        const parsed = parseSpecialGroupKey(rawKey)
        if (parsed.groupName !== name) continue
        visibilityRules.push({
          userGroup,
          visible: parsed.visible,
          description: typeof desc === 'string' ? desc : '',
        })
      }
    }

    const autoIndex = props.autoGroups.indexOf(name)

    return {
      ratio: entry?.ratio,
      topupRatio: Object.hasOwn(topupMap, name) ? String(topupMap[name]) : null,
      selectable: Object.hasOwn(usableMap, name),
      description: String(usableMap[name] ?? ''),
      incomingOverrides,
      outgoingOverrides,
      visibilityRules,
      autoIndex,
    }
  }, [
    name,
    props.registry,
    props.topupGroupRatio,
    props.userUsableGroups,
    props.groupGroupRatio,
    props.autoGroups,
    props.groupSpecialUsableGroup,
  ])

  return (
    <Sheet open={name !== null} onOpenChange={props.onOpenChange}>
      <SheetContent
        side='right'
        className={sideDrawerContentClassName('sm:max-w-lg')}
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {t('Group details')}
            {name ? `: ${display.adminLabel(name)}` : ''}
          </SheetTitle>
          <SheetDescription>
            {t('Everything configured for this group, in one place.')}
          </SheetDescription>
        </SheetHeader>

        {detail && (
          <div className={sideDrawerFormClassName('gap-5')}>
            <section className='space-y-2'>
              <h3 className='text-sm font-semibold'>{t('Overview')}</h3>
              <dl className='space-y-1.5 text-sm'>
                <div className='flex justify-between'>
                  <dt className='text-muted-foreground'>{t('Ratio')}</dt>
                  <dd className='font-medium'>{detail.ratio ?? '-'}</dd>
                </div>
                <div className='flex justify-between'>
                  <dt className='text-muted-foreground'>{t('Top-up ratio')}</dt>
                  <dd className='font-medium'>
                    {detail.topupRatio ?? t('Not set')}
                  </dd>
                </div>
                <div className='flex justify-between'>
                  <dt className='text-muted-foreground'>
                    {t('User selectable')}
                  </dt>
                  <dd className='font-medium'>
                    {detail.selectable ? t('Yes') : t('No')}
                  </dd>
                </div>
                {detail.selectable && detail.description && (
                  <div className='flex justify-between gap-4'>
                    <dt className='text-muted-foreground'>
                      {t('Description')}
                    </dt>
                    <dd className='text-right font-medium'>
                      {detail.description}
                    </dd>
                  </div>
                )}
                <div className='flex justify-between'>
                  <dt className='text-muted-foreground'>
                    {t('Auto group order')}
                  </dt>
                  <dd className='font-medium'>
                    {detail.autoIndex >= 0
                      ? t('Position {{position}}', {
                          position: detail.autoIndex + 1,
                        })
                      : t('Not included')}
                  </dd>
                </div>
              </dl>
            </section>

            <section className='space-y-2'>
              <h3 className='text-sm font-semibold'>
                {t('Ratio overrides when billed as this group')}
              </h3>
              {detail.incomingOverrides.length === 0 ? (
                <p className='text-muted-foreground text-sm'>{t('None')}</p>
              ) : (
                <ul className='space-y-1 text-sm'>
                  {detail.incomingOverrides.map((item) => (
                    <li
                      key={item.userGroup}
                      className='flex justify-between rounded-md border px-3 py-1.5'
                    >
                      <span>
                        {t('Users in {{group}}', {
                          group: display.adminLabel(item.userGroup),
                        })}
                      </span>
                      <span className='font-medium'>{item.ratio}</span>
                    </li>
                  ))}
                </ul>
              )}
            </section>

            <section className='space-y-2'>
              <h3 className='text-sm font-semibold'>
                {t('Ratio overrides for users of this group')}
              </h3>
              {detail.outgoingOverrides.length === 0 ? (
                <p className='text-muted-foreground text-sm'>{t('None')}</p>
              ) : (
                <ul className='space-y-1 text-sm'>
                  {detail.outgoingOverrides.map((item) => (
                    <li
                      key={item.targetGroup}
                      className='flex justify-between rounded-md border px-3 py-1.5'
                    >
                      <span>
                        {t('When billed as {{group}}', {
                          group: display.adminLabel(item.targetGroup),
                        })}
                      </span>
                      <span className='font-medium'>{item.ratio}</span>
                    </li>
                  ))}
                </ul>
              )}
            </section>

            <section className='space-y-2'>
              <h3 className='text-sm font-semibold'>
                {t('Special visibility rules')}
              </h3>
              {detail.visibilityRules.length === 0 ? (
                <p className='text-muted-foreground text-sm'>{t('None')}</p>
              ) : (
                <ul className='space-y-1 text-sm'>
                  {detail.visibilityRules.map((rule) => (
                    <li
                      key={`${rule.userGroup}-${rule.visible}`}
                      className='flex items-center justify-between rounded-md border px-3 py-1.5'
                    >
                      <span>
                        {rule.visible
                          ? t('Extra visible to {{group}}', {
                              group: display.adminLabel(rule.userGroup),
                            })
                          : t('Hidden from {{group}}', {
                              group: display.adminLabel(rule.userGroup),
                            })}
                      </span>
                      <StatusBadge
                        variant={rule.visible ? 'info' : 'danger'}
                        copyable={false}
                      >
                        {rule.visible ? t('Visible') : t('Hidden')}
                      </StatusBadge>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
