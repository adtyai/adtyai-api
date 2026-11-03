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
import { X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertAction, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  convertDetectedLanguage,
  INTERFACE_LANGUAGE_OPTIONS,
  normalizeInterfaceLanguage,
  toIntlLocale,
} from '@/i18n/languages'

import { homePathFor } from './home-langs'
import { readHomeLang, rememberHomeLang } from './home-language'

const HINT_DISMISSED_KEY = 'adtyai:lang-hint-dismissed'

/**
 * The browser's first preferred language when it is one of the six
 * non-English home languages; null when it prefers English or another one.
 */
function browserHomeLanguage(): string | null {
  const tags = window.navigator.languages?.length
    ? window.navigator.languages
    : [window.navigator.language]
  for (const tag of tags) {
    if (!tag) continue
    // zh-* keeps its region (zh-TW vs zh-CN); fr-FR becomes fr.
    const base = tag.toLowerCase().startsWith('zh') ? tag : tag.split('-')[0]
    const code = normalizeInterfaceLanguage(convertDetectedLanguage(base))
    if (code === 'en' && !tag.toLowerCase().startsWith('en')) continue
    return homePathFor(code) === '/' ? null : code
  }
  return null
}

/**
 * One-time suggestion on `/` for first-time visitors whose browser prefers
 * another of our languages (adty task 9.3). Browser only: it decides in an
 * effect, so the prerendered page never contains it.
 */
export function LanguageHint() {
  const { t } = useTranslation()
  const [code, setCode] = useState<string | null>(null)

  useEffect(() => {
    let seen = true
    try {
      seen = Boolean(
        readHomeLang() || window.localStorage.getItem(HINT_DISMISSED_KEY)
      )
    } catch {
      // Storage unavailable: treat as seen, so the hint never nags.
    }
    if (!seen) setCode(browserHomeLanguage())
  }, [])

  if (!code) return null
  const label =
    INTERFACE_LANGUAGE_OPTIONS.find((lang) => lang.code === code)?.label ?? code

  const dismiss = () => {
    try {
      window.localStorage.setItem(HINT_DISMISSED_KEY, '1')
    } catch {
      // Private mode: the hint may come back next time.
    }
    setCode(null)
  }

  return (
    <div className='mx-auto max-w-6xl px-4 pt-20 md:px-6'>
      <Alert role='status'>
        <AlertDescription>
          <a
            href={homePathFor(code)}
            lang={toIntlLocale(code)}
            onClick={() => rememberHomeLang(code)}
          >
            {t('Switch to {{language}}', { language: label })}
          </a>
        </AlertDescription>
        <AlertAction>
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={dismiss}
            aria-label={t('Close')}
          >
            <X aria-hidden />
          </Button>
        </AlertAction>
      </Alert>
    </div>
  )
}
