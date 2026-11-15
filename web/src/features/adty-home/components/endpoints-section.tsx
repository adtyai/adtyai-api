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

import { SectionIntro } from './section-intro'

const BEARER = 'Authorization: Bearer sk-…'

/**
 * Four protocol shapes served from one host. The old price comparison card
 * lived here; the addresses are what a reader needs before signing up.
 */
export function EndpointsSection(props: { baseUrl: string }) {
  const { t } = useTranslation()
  const rows = [
    { label: t('OpenAI compatible'), url: `${props.baseUrl}/v1`, auth: BEARER },
    { label: t('OpenAI Responses'), url: `${props.baseUrl}/v1`, auth: BEARER },
    { label: t('Anthropic Messages'), url: props.baseUrl, auth: 'x-api-key' },
    { label: t('Gemini native'), url: props.baseUrl, auth: 'x-goog-api-key' },
  ]

  return (
    <section
      id='endpoints'
      aria-labelledby='endpoints-title'
      className='border-t'
    >
      <div className='mx-auto max-w-[1180px] px-6 py-24 md:px-10 md:py-32'>
        <SectionIntro
          headingId='endpoints-title'
          eyebrow={t('Endpoints')}
          title={t('Four shapes.')}
          mutedTitle={t('One host.')}
          lead={t('Send the auth header your SDK already sends.')}
        />
        <div className='-mx-6 mt-14 overflow-x-auto px-6 md:mx-0 md:mt-16 md:px-0'>
          <table className='w-full min-w-[34rem] border-collapse text-[15px]'>
            <thead>
              <tr>
                {[t('API'), t('Base URL'), t('Auth')].map((heading) => (
                  <th
                    key={heading}
                    scope='col'
                    className='text-neutral border-border border-b pb-4 text-left font-mono text-[11px] font-semibold tracking-[0.14em] uppercase'
                  >
                    {heading}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.label} className='hover:bg-muted/40'>
                  <td className='border-border w-[38%] border-b py-5 pr-6 font-semibold'>
                    {row.label}
                  </td>
                  <td className='border-border text-muted-foreground border-b py-5 pr-6 font-mono text-[13.5px]'>
                    {row.url}
                  </td>
                  <td className='border-border text-muted-foreground border-b py-5 font-mono text-[13.5px]'>
                    {row.auth}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  )
}
