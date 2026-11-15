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

import { CodeCard } from './code-card'
import { SectionIntro } from './section-intro'

/**
 * The migration block: the one change a reader has to make, shown as the same
 * client snippets the console uses. Replaces the old step cards.
 */
export function MigrationSection(props: { baseUrl: string }) {
  const { t } = useTranslation()

  return (
    <section id='migrate' aria-labelledby='migrate-title' className='border-t'>
      <div className='mx-auto max-w-[1180px] px-6 py-24 md:px-10 md:py-32'>
        <SectionIntro
          headingId='migrate-title'
          eyebrow={t('The migration')}
          title={t('It’s one line.')}
          mutedTitle={t('Nothing else moves.')}
          lead={t(
            'Point the base URL at us and keep everything else: your SDK, your prompts, your toolchain.'
          )}
        />
        <div className='mt-14 md:mt-16'>
          <CodeCard baseUrl={props.baseUrl} />
        </div>
      </div>
    </section>
  )
}
