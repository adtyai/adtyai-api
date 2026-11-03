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
import type { AdtySeoDefaults } from './adty-seo-defaults'

type LangTexts = Partial<Record<ContentLang, string>>
type TextField = 'home_title' | 'home_description'

/** Search engines cut longer titles and descriptions; only a hint. */
const RECOMMENDED_LENGTH: Record<TextField, number> = {
  home_title: 60,
  home_description: 160,
}

function parseTexts(raw: string): LangTexts {
  try {
    const parsed: unknown = JSON.parse(raw || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as LangTexts
    }
  } catch {
    /* A broken value is shown as empty and replaced on save. */
  }
  return {}
}

/** Saved form: trimmed, without empty languages, keys in a stable order. */
function serializeTexts(texts: LangTexts): string {
  const out: LangTexts = {}
  for (const lang of CONTENT_LANGS) {
    const value = texts[lang]?.trim()
    if (value) out[lang] = value
  }
  return JSON.stringify(out)
}

function toDrafts(saved: AdtySeoDefaults) {
  return {
    home_title: parseTexts(saved.home_title),
    home_description: parseTexts(saved.home_description),
    og_image: saved.og_image,
  }
}

/** Over-length note under a field; saving is not blocked. */
function LengthHint(props: { value: string; limit: number }) {
  const { t } = useTranslation()
  const count = props.value.trim().length
  if (count <= props.limit) return null
  return (
    <p className='text-warning text-xs'>
      {t('{{count}} characters; {{limit}} or fewer is recommended.', {
        count,
        limit: props.limit,
      })}
    </p>
  )
}

/**
 * Home page SEO (adty task 9.3): title and description per language and the
 * share image. The server writes them into the seven home page URLs; a
 * language left empty uses English, and without English the title is the
 * site name.
 */
export function AdtySeoSection(props: { defaultValues: AdtySeoDefaults }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [drafts, setDrafts] = useState(() => toDrafts(props.defaultValues))

  // Reset when the saved values change (after a save refetches the options).
  const savedKey = JSON.stringify(props.defaultValues)
  useEffect(() => {
    setDrafts(toDrafts(JSON.parse(savedKey)))
  }, [savedKey])

  const save = async () => {
    for (const field of ['home_title', 'home_description'] as const) {
      const value = serializeTexts(drafts[field])
      if (value !== serializeTexts(parseTexts(props.defaultValues[field]))) {
        await updateOption.mutateAsync({
          key: `adty_seo_setting.${field}`,
          value,
        })
      }
    }
    const image = drafts.og_image.trim()
    if (image !== props.defaultValues.og_image) {
      await updateOption.mutateAsync({
        key: 'adty_seo_setting.og_image',
        value: image,
      })
    }
  }

  const setText = (field: TextField, lang: ContentLang, value: string) =>
    setDrafts((prev) => ({
      ...prev,
      [field]: { ...prev[field], [lang]: value },
    }))

  return (
    <SettingsSection title={t('Home Page SEO')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
      />
      <p className='text-muted-foreground max-w-3xl text-sm'>
        {t(
          'Written into the source of the seven home page URLs for search engines and link previews. A language left empty uses English; without English the title is the site name and there is no description. Other pages are never indexed.'
        )}
      </p>
      <div className='max-w-3xl space-y-4'>
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
          {CONTENT_LANGS.map((lang) => {
            const title = drafts.home_title[lang] ?? ''
            const description = drafts.home_description[lang] ?? ''
            return (
              <TabsContent key={lang} value={lang} className='space-y-4'>
                <div className='space-y-2'>
                  <Label htmlFor={`adty-seo-title-${lang}`}>
                    {t('Title ({{language}})', {
                      language: CONTENT_LANG_LABELS[lang],
                    })}
                  </Label>
                  <Input
                    id={`adty-seo-title-${lang}`}
                    value={title}
                    onChange={(event) =>
                      setText('home_title', lang, event.target.value)
                    }
                  />
                  <LengthHint
                    value={title}
                    limit={RECOMMENDED_LENGTH.home_title}
                  />
                </div>
                <div className='space-y-2'>
                  <Label htmlFor={`adty-seo-description-${lang}`}>
                    {t('Description ({{language}})', {
                      language: CONTENT_LANG_LABELS[lang],
                    })}
                  </Label>
                  <Textarea
                    id={`adty-seo-description-${lang}`}
                    rows={3}
                    value={description}
                    onChange={(event) =>
                      setText('home_description', lang, event.target.value)
                    }
                  />
                  <LengthHint
                    value={description}
                    limit={RECOMMENDED_LENGTH.home_description}
                  />
                </div>
              </TabsContent>
            )
          })}
        </Tabs>
        <div className='space-y-2'>
          <Label htmlFor='adty-seo-og-image'>{t('Share image')}</Label>
          <Input
            id='adty-seo-og-image'
            value={drafts.og_image}
            placeholder='/adty-og.png'
            onChange={(event) =>
              setDrafts((prev) => ({ ...prev, og_image: event.target.value }))
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Shown in link previews, 1200 by 630 pixels. A path starting with / is served by this site.'
            )}
          </p>
        </div>
      </div>
    </SettingsSection>
  )
}
