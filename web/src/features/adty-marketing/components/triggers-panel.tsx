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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { CONTENT_LANG_LABELS, CONTENT_LANGS } from '@/lib/adty-lang'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getMarketingTriggers,
  marketingKeys,
  saveMarketingTriggers,
  type MarketingTriggers,
  type TriggerKey,
  type TriggerRule,
} from '../api'
import { CAMPAIGN_VARIABLES, hasVersion } from '../lib/campaign'

const KEYS: TriggerKey[] = ['unpaid', 'low_balance', 'winback']

/** Automated emails (adty task 12.3): each rule has its own switch, threshold and templates. */
export function TriggersPanel(props: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: marketingKeys.triggers,
    queryFn: getMarketingTriggers,
  })
  const [drafts, setDrafts] = useState<MarketingTriggers | null>(null)
  useEffect(() => {
    if (query.data) setDrafts(structuredClone(query.data))
  }, [query.data])
  const save = useMutation({
    mutationFn: (body: MarketingTriggers) => saveMarketingTriggers(body),
    onSuccess: (next) => {
      queryClient.setQueryData(marketingKeys.triggers, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })
  if (!drafts) return <LoadingState />

  const info: Record<
    TriggerKey,
    { title: string; hint: string; field: string }
  > = {
    unpaid: {
      title: t('Used the API but never topped up'),
      hint: t('Sent once, N days after sign-up.'),
      field: t('Days after sign-up'),
    },
    low_balance: {
      title: t('Balance running low'),
      hint: t(
        'Paying users below the threshold. Skipped when the account low-balance notice already covers them.'
      ),
      field: t('Balance below (USD)'),
    },
    winback: {
      title: t('Win back paying users'),
      hint: t('Paying users with no API calls in the last N days.'),
      field: t('Days without calls'),
    },
  }
  const setRule = (key: TriggerKey, patch: Partial<TriggerRule>) =>
    setDrafts((prev) =>
      prev ? { ...prev, [key]: { ...prev[key], ...patch } } : prev
    )
  const disabled = !props.canWrite || save.isPending

  return (
    <form
      className='flex max-w-4xl flex-col gap-6'
      onSubmit={(event) => {
        event.preventDefault()
        save.mutate(drafts)
      }}
    >
      <p className='text-muted-foreground text-sm'>
        {t(
          'Automated emails go only to users who agreed to marketing emails, at most once per user and rule. Variables: {{variables}}.',
          { variables: CAMPAIGN_VARIABLES.join(' ') }
        )}
      </p>
      {KEYS.map((key) => {
        const rule = drafts[key]
        const englishReady = hasVersion(rule.subjects, rule.bodies, 'en')
        return (
          <fieldset
            key={key}
            className='flex flex-col gap-3 rounded-lg border p-4'
          >
            <div className='flex items-center justify-between gap-4'>
              <div className='flex flex-col'>
                <Label htmlFor={`trigger-${key}`}>{info[key].title}</Label>
                <span className='text-muted-foreground text-xs'>
                  {info[key].hint}
                </span>
                {!englishReady && (
                  <span className='text-warning text-xs'>
                    {t('Write the English version first')}
                  </span>
                )}
              </div>
              <Switch
                id={`trigger-${key}`}
                checked={rule.enabled}
                disabled={disabled || (!englishReady && !rule.enabled)}
                onCheckedChange={(enabled) => setRule(key, { enabled })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`trigger-${key}-value`}>{info[key].field}</Label>
              <Input
                id={`trigger-${key}-value`}
                type='number'
                className='w-32'
                min={key === 'low_balance' ? 0.01 : 1}
                step={key === 'low_balance' ? '0.01' : 1}
                disabled={disabled}
                value={
                  key === 'low_balance'
                    ? (rule.usd ?? '')
                    : String(rule.days ?? '')
                }
                onChange={(event) =>
                  key === 'low_balance'
                    ? setRule(key, { usd: event.target.value })
                    : setRule(key, { days: Number(event.target.value) })
                }
              />
            </div>
            <Tabs defaultValue='en' className='gap-3'>
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
                  <Input
                    aria-label={t('Subject in {{language}}', {
                      language: CONTENT_LANG_LABELS[lang],
                    })}
                    disabled={disabled}
                    value={rule.subjects[lang] ?? ''}
                    onChange={(event) =>
                      setRule(key, {
                        subjects: {
                          ...rule.subjects,
                          [lang]: event.target.value,
                        },
                      })
                    }
                  />
                  <Textarea
                    aria-label={t('Body in {{language}} (HTML)', {
                      language: CONTENT_LANG_LABELS[lang],
                    })}
                    rows={6}
                    className='font-mono text-xs'
                    disabled={disabled}
                    value={rule.bodies[lang] ?? ''}
                    onChange={(event) =>
                      setRule(key, {
                        bodies: { ...rule.bodies, [lang]: event.target.value },
                      })
                    }
                  />
                </TabsContent>
              ))}
            </Tabs>
          </fieldset>
        )
      })}
      <div className='space-y-2'>
        <Label htmlFor='trigger-batch-size'>
          {t('Emails per rule per hour at most')}
        </Label>
        <Input
          id='trigger-batch-size'
          type='number'
          min={1}
          step={1}
          className='w-32'
          disabled={disabled}
          value={drafts.batch_size}
          onChange={(event) =>
            setDrafts((prev) =>
              prev ? { ...prev, batch_size: Number(event.target.value) } : prev
            )
          }
        />
      </div>
      {props.canWrite && (
        <div>
          <Button type='submit' disabled={save.isPending}>
            {t('Save Changes')}
          </Button>
        </div>
      )}
    </form>
  )
}
