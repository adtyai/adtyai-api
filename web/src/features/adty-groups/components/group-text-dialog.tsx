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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CONTENT_LANG_LABELS, CONTENT_LANGS } from '@/lib/adty-lang'
import { handleServerError } from '@/lib/handle-server-error'

import {
  adminGroupsQueryKey,
  groupDisplayQueryKey,
  updateAdminGroup,
  type AdminGroup,
  type GroupTexts,
} from '../api'

type GroupTextDialogProps = {
  group: AdminGroup | null
  onOpenChange: (open: boolean) => void
}

/** Edits a group's display name and description in the 7 content languages. */
export function GroupTextDialog(props: GroupTextDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [names, setNames] = useState<GroupTexts>({})
  const [descs, setDescs] = useState<GroupTexts>({})

  useEffect(() => {
    setNames({ ...props.group?.names })
    setDescs({ ...props.group?.descs })
  }, [props.group])

  const mutation = useMutation({
    mutationFn: () => updateAdminGroup(props.group?.id ?? '', { names, descs }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: adminGroupsQueryKey }),
        queryClient.invalidateQueries({ queryKey: groupDisplayQueryKey }),
      ])
      toast.success(t('Group names saved'))
      props.onOpenChange(false)
    },
    onError: (error) => handleServerError(error),
  })

  const englishName = names.en?.trim() ?? ''

  return (
    <Dialog
      open={props.group !== null}
      onOpenChange={props.onOpenChange}
      title={t('Edit group names')}
      description={t(
        'The group ID {{id}} never changes. Users see the name in their language, or English when it is empty.',
        { id: props.group?.id ?? '' }
      )}
      contentHeight='auto'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!englishName || mutation.isPending}
          >
            {mutation.isPending ? t('Saving...') : t('Save')}
          </Button>
        </>
      }
    >
      <Tabs defaultValue='en' className='gap-4'>
        <div className='overflow-x-auto pb-1'>
          <TabsList className='w-full min-w-max'>
            {CONTENT_LANGS.map((lang) => (
              <TabsTrigger key={lang} value={lang} className='px-2'>
                {CONTENT_LANG_LABELS[lang]}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        {CONTENT_LANGS.map((lang) => (
          <TabsContent key={lang} value={lang} className='space-y-4'>
            <div className='space-y-2'>
              <Label htmlFor={`group-name-${lang}`}>
                {lang === 'en'
                  ? t('Display name (required)')
                  : t('Display name')}
              </Label>
              <Input
                id={`group-name-${lang}`}
                value={names[lang] ?? ''}
                aria-invalid={lang === 'en' && !englishName}
                onChange={(event) =>
                  setNames((prev) => ({ ...prev, [lang]: event.target.value }))
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`group-desc-${lang}`}>{t('Description')}</Label>
              <Input
                id={`group-desc-${lang}`}
                value={descs[lang] ?? ''}
                onChange={(event) =>
                  setDescs((prev) => ({ ...prev, [lang]: event.target.value }))
                }
              />
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </Dialog>
  )
}
