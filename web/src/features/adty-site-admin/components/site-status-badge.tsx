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

import { StatusBadge } from '@/components/status-badge'

import type { SiteStatus } from '../types'

export function SiteStatusBadge(props: { status: SiteStatus }) {
  const { t } = useTranslation()
  // Literal t() calls so the i18n scanner sees every label.
  switch (props.status) {
    case 'pending':
      return <StatusBadge label={t('Pending review')} variant='warning' />
    case 'active':
      return <StatusBadge label={t('Active')} variant='success' />
    case 'paused':
      return <StatusBadge label={t('Paused')} variant='neutral' />
    case 'closing':
      return <StatusBadge label={t('Closing')} variant='danger' />
    default:
      return <StatusBadge label={t('Closed')} variant='neutral' />
  }
}
