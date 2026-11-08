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

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  CONTENT_LANG_LABELS,
  CONTENT_LANGS,
  type ContentLang,
} from '@/lib/adty-lang'

import { DraftNavigationProvider } from '../components/draft-navigation-provider'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useDraftNavigation } from '../hooks/use-draft-navigation'
import { useUpdateOption } from '../hooks/use-update-option'

export type AdtyLegalField =
  | 'user_agreement'
  | 'privacy_policy'
  | 'refund_policy'
  | 'site_agreement'
  | 'site_customer_terms'

type VersionKey = 'site_agreement_version' | 'site_customer_terms_version'
type LegalKey = AdtyLegalField | VersionKey

/** Saved option values: each is a JSON object of content language → Markdown. */
export type AdtyLegalDefaults = Record<AdtyLegalField, string> &
  Record<VersionKey, number>

type LegalTexts = Partial<Record<ContentLang, string>>

const FIELDS: AdtyLegalField[] = [
  'user_agreement',
  'privacy_policy',
  'refund_policy',
  'site_agreement',
  'site_customer_terms',
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

function toValues(saved: AdtyLegalDefaults): Record<LegalKey, string> {
  return {
    user_agreement: serializeTexts(parseTexts(saved.user_agreement)),
    privacy_policy: serializeTexts(parseTexts(saved.privacy_policy)),
    refund_policy: serializeTexts(parseTexts(saved.refund_policy)),
    site_agreement: serializeTexts(parseTexts(saved.site_agreement)),
    site_customer_terms: serializeTexts(parseTexts(saved.site_customer_terms)),
    site_agreement_version: String(saved.site_agreement_version),
    site_customer_terms_version: String(saved.site_customer_terms_version),
  }
}

function versionKeyFor(field: AdtyLegalField): VersionKey | null {
  if (field === 'site_agreement') return 'site_agreement_version'
  if (field === 'site_customer_terms') return 'site_customer_terms_version'
  return null
}

function normalizeValue(key: LegalKey, value: string) {
  return FIELDS.includes(key as AdtyLegalField)
    ? serializeTexts(parseTexts(value))
    : value
}

function LegalEditor(props: { defaultValues: AdtyLegalDefaults }) {
  const { t } = useTranslation()
  const labels: Record<AdtyLegalField, string> = {
    user_agreement: t('User Agreement'),
    privacy_policy: t('Privacy Policy'),
    refund_policy: t('Refund Policy'),
    site_agreement: t('Site Agreement'),
    site_customer_terms: t('Site Customer Terms'),
  }
  const updateOption = useUpdateOption()
  const [field, setField] = useState<AdtyLegalField>('user_agreement')
  const [editor, setEditor] = useState(() => ({
    values: toValues(props.defaultValues),
    baseline: toValues(props.defaultValues),
  }))
  const [saving, setSaving] = useState(false)
  const versionKey = versionKeyFor(field)
  const version = versionKey ? editor.values[versionKey] : ''
  const validVersion =
    !versionKey ||
    (/^(0|[1-9]\d*)$/.test(version) && Number.isSafeInteger(Number(version)))
  const dirty = (Object.keys(editor.values) as LegalKey[]).some(
    (key) => normalizeValue(key, editor.values[key]) !== editor.baseline[key]
  )
  useDraftNavigation(dirty)

  const savedKey = JSON.stringify(props.defaultValues)
  useEffect(() => {
    const next = toValues(JSON.parse(savedKey))
    setEditor((current) => {
      const values = { ...current.values }
      for (const key of Object.keys(next) as LegalKey[]) {
        if (normalizeValue(key, values[key]) === current.baseline[key]) {
          values[key] = next[key]
        }
      }
      return { values, baseline: next }
    })
  }, [savedKey])

  const save = async () => {
    if (!validVersion || saving) return
    const changes: Array<{ key: LegalKey; value: string }> = []
    for (const key of [field, versionKey]) {
      if (!key) continue
      const value = normalizeValue(key, editor.values[key])
      if (value !== editor.baseline[key]) changes.push({ key, value })
    }
    setSaving(true)
    try {
      for (const { key, value } of changes) {
        await updateOption.mutateAsync({
          key: `adty_legal_setting.${key}`,
          value,
        })
        setEditor((current) => ({
          ...current,
          baseline: { ...current.baseline, [key]: value },
        }))
      }
    } catch {
      // The mutation reports the error; remaining drafts stay available to retry.
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsSection title={t('Multilingual Legal Pages')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={saving}
        isSaveDisabled={!validVersion}
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
        {versionKey && (
          <div className='space-y-2'>
            <Label htmlFor='adty-legal-version'>{t('Document version')}</Label>
            <Input
              id='adty-legal-version'
              type='number'
              min={0}
              step={1}
              value={version}
              aria-invalid={!validVersion}
              onChange={(event) => {
                const value = event.target.value
                setEditor((current) => ({
                  ...current,
                  values: { ...current.values, [versionKey]: value },
                }))
              }}
            />
            <p className='text-muted-foreground text-sm'>
              {t('Increase the version after changing a published agreement.')}
            </p>
            {!validVersion && (
              <p className='text-destructive text-sm' role='alert'>
                {t('Version must be a non-negative integer.')}
              </p>
            )}
          </div>
        )}
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
                value={parseTexts(editor.values[field])[lang] ?? ''}
                onChange={(event) =>
                  setEditor((current) => ({
                    ...current,
                    values: {
                      ...current.values,
                      [field]: JSON.stringify({
                        ...parseTexts(current.values[field]),
                        [lang]: event.target.value,
                      }),
                    },
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

/** Five multilingual legal documents, with versioned site agreements. */
export function AdtyLegalSection(props: { defaultValues: AdtyLegalDefaults }) {
  return (
    <DraftNavigationProvider>
      <LegalEditor {...props} />
    </DraftNavigationProvider>
  )
}
