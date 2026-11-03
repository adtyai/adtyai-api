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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  adminGroupsQueryKey,
  groupDisplayQueryKey,
  migrateAdminGroup,
  type AdminGroup,
} from '../api'
import { useGroupDisplay } from '../hooks/use-group-display'

type GroupMigrateDialogProps = {
  group: AdminGroup | null
  groups: AdminGroup[]
  onOpenChange: (open: boolean) => void
}

/** Moves every user, key and channel of one group to another group. */
export function GroupMigrateDialog(props: GroupMigrateDialogProps) {
  const { t } = useTranslation()
  const display = useGroupDisplay()
  const queryClient = useQueryClient()
  const [target, setTarget] = useState<string | null>(null)
  const from = props.group

  const mutation = useMutation({
    mutationFn: () => migrateAdminGroup(from?.id ?? '', target ?? ''),
    onSuccess: async (refs) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: adminGroupsQueryKey }),
        queryClient.invalidateQueries({ queryKey: groupDisplayQueryKey }),
      ])
      toast.success(
        t('Moved {{users}} users, {{tokens}} keys and {{channels}} channels', {
          users: refs.users,
          tokens: refs.tokens,
          channels: refs.channels,
        })
      )
      setTarget(null)
      props.onOpenChange(false)
    },
    onError: (error) => handleServerError(error),
  })

  const options = props.groups
    .filter((group) => group.id !== from?.id)
    .map((group) => ({ value: group.id, label: display.adminLabel(group.id) }))

  return (
    <Dialog
      open={from !== null}
      onOpenChange={(open) => {
        if (!open) setTarget(null)
        props.onOpenChange(open)
      }}
      title={t('Migrate group')}
      description={t(
        'Move all users, keys and channels of {{group}} to another group. Then the group can be deleted.',
        { group: from ? display.adminLabel(from.id) : '' }
      )}
      contentHeight='auto'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!target || mutation.isPending}
          >
            {t('Migrate')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label>{t('Target group')}</Label>
        <Combobox
          className='w-full'
          options={options}
          value={target}
          onValueChange={setTarget}
          placeholder={t('Select a group')}
          aria-label={t('Target group')}
        />
      </div>
    </Dialog>
  )
}
