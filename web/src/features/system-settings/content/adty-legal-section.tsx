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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  CONTENT_LANG_LABELS,
  CONTENT_LANGS,
  type ContentLang,
} from '@/lib/adty-lang'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export type AdtyLegalField =
  | 'user_agreement'
  | 'privacy_policy'
  | 'refund_policy'

/** Saved option values: each is a JSON object of content language → Markdown. */
export type AdtyLegalDefaults = Record<AdtyLegalField, string>

type LegalTexts = Partial<Record<ContentLang, string>>

const FIELDS: AdtyLegalField[] = [
  'user_agreement',
  'privacy_policy',
  'refund_policy',
]

function parseTexts(raw: string): LegalTexts {
  try {
    const parsed: unknown = JSON.parse(raw || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as LegalTexts
    }
  } catch {
    /* A broken value is shown as empty and replaced on save. */
  }
  return {}
}

/** Saved form: trimmed, without empty languages, keys in a stable order. */
function serializeTexts(texts: LegalTexts): string {
  const out: LegalTexts = {}
  for (const lang of CONTENT_LANGS) {
    const value = texts[lang]?.trim()
    if (value) out[lang] = texts[lang]
  }
  return JSON.stringify(out)
}

function toDrafts(saved: AdtyLegalDefaults) {
  return {
    user_agreement: parseTexts(saved.user_agreement),
    privacy_policy: parseTexts(saved.privacy_policy),
    refund_policy: parseTexts(saved.refund_policy),
  }
}

/** Multilingual legal texts (adty task 8.6): one Markdown per document and content language. */
export function AdtyLegalSection(props: { defaultValues: AdtyLegalDefaults }) {
  const { t } = useTranslation()
  const labels: Record<AdtyLegalField, string> = {
    user_agreement: t('User Agreement'),
    privacy_policy: t('Privacy Policy'),
    refund_policy: t('Refund Policy'),
  }
  const updateOption = useUpdateOption()
  const [field, setField] = useState<AdtyLegalField>('user_agreement')
  const [drafts, setDrafts] = useState(() => toDrafts(props.defaultValues))

  // Reset when the saved values change (after a save refetches the options).
  const savedKey = JSON.stringify(props.defaultValues)
  useEffect(() => {
    setDrafts(toDrafts(JSON.parse(savedKey)))
  }, [savedKey])

  const save = async () => {
    for (const item of FIELDS) {
      const value = serializeTexts(drafts[item])
      if (value !== serializeTexts(parseTexts(props.defaultValues[item]))) {
        await updateOption.mutateAsync({
          key: `adty_legal_setting.${item}`,
          value,
        })
      }
    }
  }

  return (
    <SettingsSection title={t('Multilingual Legal Pages')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
      />
      <p className='text-muted-foreground max-w-3xl text-sm'>
        {t(
          'Readers see the text in their language, then English. When both are empty, the user agreement and privacy policy fall back to the text under Site → System Info, which also decides whether sign-up asks for consent. The refund policy has no fallback and is shown only when it is enabled in the site settings.'
        )}
      </p>
      <div className='max-w-3xl space-y-4'>
        <div className='space-y-2'>
          <Label htmlFor='adty-legal-doc'>{t('Document')}</Label>
          <NativeSelect
            id='adty-legal-doc'
            value={field}
            onChange={(event) => setField(event.target.value as AdtyLegalField)}
          >
            {FIELDS.map((item) => (
              <NativeSelectOption key={item} value={item}>
                {labels[item]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
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
            <TabsContent key={lang} value={lang} className='space-y-2'>
              <Label htmlFor={`adty-legal-${lang}`}>
                {t('{{document}} in {{language}} (Markdown)', {
                  document: labels[field],
                  language: CONTENT_LANG_LABELS[lang],
                })}
              </Label>
              <Textarea
                id={`adty-legal-${lang}`}
                rows={14}
                className='font-mono text-xs'
                value={drafts[field][lang] ?? ''}
                onChange={(event) =>
                  setDrafts((prev) => ({
                    ...prev,
                    [field]: { ...prev[field], [lang]: event.target.value },
                  }))
                }
              />
            </TabsContent>
          ))}
        </Tabs>
      </div>
    </SettingsSection>
  )
}
