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

import { useAdtySite } from '@/features/adty-site/api'

import { LegalDocument } from './legal-document'

/** adty: shown only while the site setting refund_policy_enabled is on. */
export function RefundPolicy() {
  const { t } = useTranslation()
  const site = useAdtySite()
  return (
    <LegalDocument
      title={t('Refund Policy')}
      doc='refund-policy'
      pending={site.isPending}
      hidden={!site.isPending && site.data?.refund_policy_enabled !== true}
      emptyMessage={t(
        'The administrator has not configured a refund policy yet.'
      )}
    />
  )
}
