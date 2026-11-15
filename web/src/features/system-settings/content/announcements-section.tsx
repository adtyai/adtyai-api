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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { NotificationManager } from '@/features/adty-notifications/components/notification-manager'
import { handleServerError } from '@/lib/handle-server-error'

import { SettingsSwitchField } from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type AnnouncementsSectionProps = {
  enabled: boolean
  /** 旧 `console_setting.announcements` 的内容。已不再读：公告改由发布记录承载。 */
  data?: string
}

/**
 * 公告设置只做两件事：控制公开面板的开关，以及发布/编辑公告。
 *
 * 这里原来是两套并存的东西——新的 `NotificationManager` 加上一个直接编辑
 * `console_setting.announcements` 的旧表格。两套都能发公告，读者却只有一份，
 * 于是同一件事有两个来源。旧表格已经删掉，公告只走发布记录
 * （`adty_notification_publications`）。
 *
 * `console_setting.announcements` 这个 option **仍然保留在库里**，它是回滚源：
 * 启动时的迁移把它的每条搬进发布记录（幂等），删了就没法退回去。
 */
export function AnnouncementsSection({ enabled }: AnnouncementsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const handleToggleEnabled = async (checked: boolean) => {
    try {
      await updateOption.mutateAsync({
        key: 'console_setting.announcements_enabled',
        value: checked,
      })
      toast.success(t('Setting saved'))
    } catch (error) {
      handleServerError(error, t('Failed to update setting'))
    }
  }

  return (
    <>
      <SettingsSection title={t('Announcements')}>
        <SettingsSwitchField
          checked={enabled}
          onCheckedChange={handleToggleEnabled}
          label={t('Enabled')}
          description={t(
            'When disabled, published announcements are hidden from the public dashboard.'
          )}
        />
      </SettingsSection>
      <NotificationManager mode='root' />
    </>
  )
}
