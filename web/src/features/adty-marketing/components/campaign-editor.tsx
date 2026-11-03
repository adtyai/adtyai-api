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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DateTimePicker } from '@/components/datetime-picker'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { toIntlLocale } from '@/i18n/languages'
import { CONTENT_LANG_LABELS, CONTENT_LANGS } from '@/lib/adty-lang'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getCampaignPlan,
  marketingKeys,
  saveCampaign,
  sendCampaign,
  sendCampaignTest,
  type CampaignInput,
  type CampaignPlan,
} from '../api'
import { CAMPAIGN_VARIABLES, hasVersion } from '../lib/campaign'
import { AudienceFilters } from './audience-filters'

type CampaignEditorProps = {
  campaignId: number | null
  initial: CampaignInput
  canWrite: boolean
  canSend: boolean
  onClose: () => void
}

/** Writes a campaign or template: 7 language versions, filters, test send and confirmed send. */
export function CampaignEditor(props: CampaignEditorProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [id, setId] = useState(props.campaignId)
  const [draft, setDraft] = useState(props.initial)
  const [testTo, setTestTo] = useState('')
  const [testLang, setTestLang] = useState('en')
  const [plan, setPlan] = useState<CampaignPlan | null>(null)
  const readOnly = !props.canWrite
  const englishReady = hasVersion(draft.subjects, draft.bodies, 'en')

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: marketingKeys.all })
  const save = useMutation({
    mutationFn: () => saveCampaign(id, draft),
    onSuccess: (saved) => {
      setId(saved.id)
      toast.success(t('Saved successfully'))
      void refresh()
    },
    onError: (err) => handleServerError(err),
  })
  const test = useMutation({
    mutationFn: async () => {
      const saved = await saveCampaign(id, draft)
      setId(saved.id)
      await sendCampaignTest(saved.id, { to: testTo.trim(), lang: testLang })
    },
    onSuccess: () => toast.success(t('Test email sent')),
    onError: (err) => handleServerError(err),
  })
  const prepare = useMutation({
    mutationFn: async () => {
      const saved = await saveCampaign(id, draft)
      setId(saved.id)
      return getCampaignPlan(saved.id)
    },
    onSuccess: setPlan,
    onError: (err) => handleServerError(err),
  })
  const send = useMutation({
    mutationFn: () => sendCampaign(id ?? 0, plan?.total ?? 0),
    onSuccess: () => {
      toast.success(
        draft.scheduled_at > 0 ? t('Campaign scheduled') : t('Campaign sent')
      )
      setPlan(null)
      void refresh()
      props.onClose()
    },
    onError: (err) => handleServerError(err),
  })

  const setText = (field: 'subjects' | 'bodies', lang: string, value: string) =>
    setDraft((prev) => ({
      ...prev,
      [field]: { ...prev[field], [lang]: value },
    }))

  return (
    <div className='flex max-w-4xl flex-col gap-6'>
      <div className='flex flex-wrap items-end gap-4'>
        <div className='min-w-64 flex-1 space-y-2'>
          <Label htmlFor='campaign-name'>
            {t('Name (only admins see it)')}
          </Label>
          <Input
            id='campaign-name'
            value={draft.name}
            disabled={readOnly}
            maxLength={128}
            onChange={(event) =>
              setDraft((prev) => ({ ...prev, name: event.target.value }))
            }
          />
        </div>
        {!draft.is_template && (
          <div className='space-y-2'>
            <Label>{t('Send at (empty = right away)')}</Label>
            <DateTimePicker
              value={
                draft.scheduled_at > 0
                  ? new Date(draft.scheduled_at * 1000)
                  : undefined
              }
              onChange={(date) =>
                setDraft((prev) => ({
                  ...prev,
                  scheduled_at: date ? Math.floor(date.getTime() / 1000) : 0,
                }))
              }
            />
          </div>
        )}
      </div>

      <div className='space-y-2'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Readers get their own language, or English when that version is empty. Variables: {{variables}}. The unsubscribe link and footer are added automatically.',
            { variables: CAMPAIGN_VARIABLES.join(' ') }
          )}
        </p>
        <Tabs defaultValue='en' className='gap-4'>
          <div className='overflow-x-auto pb-1'>
            <TabsList className='w-full min-w-max'>
              {CONTENT_LANGS.map((lang) => (
                <TabsTrigger key={lang} value={lang} className='px-2'>
                  {CONTENT_LANG_LABELS[lang]}
                  {hasVersion(draft.subjects, draft.bodies, lang) && ' ✓'}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          {CONTENT_LANGS.map((lang) => (
            <TabsContent key={lang} value={lang} className='space-y-3'>
              <div className='space-y-2'>
                <Label htmlFor={`campaign-subject-${lang}`}>
                  {t('Subject in {{language}}', {
                    language: CONTENT_LANG_LABELS[lang],
                  })}
                </Label>
                <Input
                  id={`campaign-subject-${lang}`}
                  value={draft.subjects[lang] ?? ''}
                  disabled={readOnly}
                  maxLength={300}
                  onChange={(event) =>
                    setText('subjects', lang, event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor={`campaign-body-${lang}`}>
                  {t('Body in {{language}} (HTML)', {
                    language: CONTENT_LANG_LABELS[lang],
                  })}
                </Label>
                <Textarea
                  id={`campaign-body-${lang}`}
                  rows={12}
                  className='font-mono text-xs'
                  value={draft.bodies[lang] ?? ''}
                  disabled={readOnly}
                  onChange={(event) =>
                    setText('bodies', lang, event.target.value)
                  }
                />
              </div>
            </TabsContent>
          ))}
        </Tabs>
        {!englishReady && (
          <p className='text-warning text-xs'>
            {t('The English version is required; it is the fallback.')}
          </p>
        )}
      </div>

      {!draft.is_template && (
        <AudienceFilters
          value={draft.audience}
          disabled={readOnly}
          onChange={(audience) => setDraft((prev) => ({ ...prev, audience }))}
        />
      )}

      <div className='flex flex-wrap items-center gap-2'>
        {props.canWrite && (
          <Button
            variant='outline'
            disabled={save.isPending || !draft.name.trim()}
            onClick={() => save.mutate()}
          >
            {draft.is_template ? t('Save template') : t('Save draft')}
          </Button>
        )}
        <Button variant='ghost' onClick={props.onClose}>
          {t('Back')}
        </Button>
      </div>

      {props.canSend && props.canWrite && !draft.is_template && (
        <div className='flex flex-col gap-3 rounded-lg border p-4'>
          <div className='flex flex-wrap items-end gap-2'>
            <div className='space-y-2'>
              <Label htmlFor='campaign-test-to'>{t('Send a test to')}</Label>
              <Input
                id='campaign-test-to'
                type='email'
                className='w-64'
                value={testTo}
                onChange={(event) => setTestTo(event.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='campaign-test-lang'>{t('Language')}</Label>
              <NativeSelect
                id='campaign-test-lang'
                value={testLang}
                onChange={(event) => setTestLang(event.target.value)}
              >
                {CONTENT_LANGS.map((lang) => (
                  <NativeSelectOption key={lang} value={lang}>
                    {CONTENT_LANG_LABELS[lang]}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <Button
              variant='outline'
              disabled={test.isPending || !testTo.trim() || !englishReady}
              onClick={() => test.mutate()}
            >
              {t('Send test')}
            </Button>
          </div>
          <div>
            <Button
              disabled={
                prepare.isPending || !englishReady || !draft.name.trim()
              }
              onClick={() => prepare.mutate()}
            >
              {draft.scheduled_at > 0 ? t('Schedule…') : t('Send…')}
            </Button>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={plan !== null}
        onOpenChange={(open) => !open && setPlan(null)}
        title={t('Send this campaign?')}
        desc={
          <div className='space-y-2 text-sm' data-testid='send-plan'>
            <p>
              {t('{{total}} recipients across {{sites}} site(s).', {
                total: formatNumber(plan?.total ?? 0, locale),
                sites: plan?.sites ?? 0,
              })}
            </p>
            <ul className='list-disc pl-5'>
              {Object.entries(plan?.versions ?? {}).map(([lang, count]) => (
                <li key={lang}>
                  {CONTENT_LANG_LABELS[
                    lang as keyof typeof CONTENT_LANG_LABELS
                  ] ?? lang}
                  : {formatNumber(count, locale)}
                </li>
              ))}
            </ul>
          </div>
        }
        confirmText={t('Send')}
        disabled={!plan || plan.total === 0}
        isLoading={send.isPending}
        handleConfirm={() => send.mutate()}
      />
    </div>
  )
}
