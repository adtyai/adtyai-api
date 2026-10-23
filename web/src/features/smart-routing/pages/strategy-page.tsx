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

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { previewWeightScheme, rollbackToOfficial, simulateRouting } from '../api'
import {
  usePutRoutingConfig,
  useRoutingConfig,
  useWeightSchemes,
} from '../hooks/use-routing-config'
import { routingErrorText } from '../lib/error-text'
import { previewIntentFor } from '../lib/preview-intent'
import type { SimulateResult } from '../types'

const MODES = [
  {
    id: 'off',
    label: 'Routing mode: off',
    hint: 'Off. Selection uses official priority and weight.',
  },
  {
    id: 'observe',
    label: 'Routing mode: observe',
    hint: 'Observe. Selection stays official. The log records what smart routing would have picked.',
  },
  {
    id: 'active',
    label: 'Routing mode: active',
    hint: 'Active. Smart weights apply only inside the current priority band.',
  },
] as const

const PREFERENCES = [
  { name: 'balanced_default', label: 'Balanced preference' },
  { name: 'prefer_save', label: 'Save-money preference' },
  { name: 'prefer_fast', label: 'Faster preference' },
  { name: 'prefer_stable', label: 'More-stable preference' },
] as const

export function StrategyPage() {
  const { t } = useTranslation()
  const isRoot = useAuthStore((s) => s.auth.user?.role === ROLE.SUPER_ADMIN)
  const configQuery = useRoutingConfig()
  const schemesQuery = useWeightSchemes()
  const putConfig = usePutRoutingConfig()
  const [model, setModel] = useState('')
  const [preview, setPreview] = useState<SimulateResult | null>(null)
  const config = configQuery.data
  const ready = Boolean(config) && !configQuery.isError

  const setMode = async (mode: (typeof MODES)[number]['id']) => {
    if (!ready || !isRoot || mode === config?.mode) return
    const res = await putConfig.mutateAsync({ mode })
    if (!res.success) {
      toast.error(routingErrorText(t, res, t('Save failed')))
      return
    }
    toast.success(t('Saved successfully'))
  }

  const usePreference = async (name: string) => {
    if (!ready || !isRoot || !config) return
    const scheme = (schemesQuery.data ?? []).find((s) => s.name === name)
    if (!scheme) {
      toast.error(t('Failed to load'))
      return
    }
    if (config.active_weight_scheme === name) return
    const intent = previewIntentFor({
      activeScheme: config.active_weight_scheme,
      currentWeights: config.weights,
      nextWeights: scheme.weights,
      schemeName: name,
    })
    const token = await previewWeightScheme({
      weights: scheme.weights,
      scheme_name: intent.scheme_name,
      kind: intent.kind,
    })
    if (!token.preview_token) {
      toast.error(token.error || t('Save failed'))
      return
    }
    const res = await putConfig.mutateAsync({
      weights: scheme.weights,
      active_weight_scheme: name,
      preview_token: token.preview_token,
    })
    if (!res.success) {
      toast.error(routingErrorText(t, res, t('Save failed')))
      return
    }
    toast.success(t('Saved successfully'))
  }

  const rollback = async () => {
    if (!isRoot) return
    const res = await rollbackToOfficial()
    if (!res.success) {
      toast.error(routingErrorText(t, res, t('Rollback failed')))
      return
    }
    toast.success(t('Rolled back to official channel weights'))
  }

  const runPreview = async () => {
    if (!model.trim()) {
      toast.error(t('Enter a model name'))
      return
    }
    setPreview(await simulateRouting({ model: model.trim() }))
  }

  return (
    <div className='flex flex-col gap-4 overflow-auto pb-4'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Routing mode')}</CardTitle>
          <CardDescription>
            {t(
              'The page shows the server setting. A failed load does not change it.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          {configQuery.isError ? (
            <p className='text-destructive text-sm'>{t('Failed to load')}</p>
          ) : null}
          <div className='flex flex-wrap gap-2'>
            {MODES.map((mode) => (
              <Button
                key={mode.id}
                type='button'
                size='sm'
                variant={config?.mode === mode.id ? 'default' : 'outline'}
                disabled={!ready || !isRoot || putConfig.isPending}
                onClick={() => void setMode(mode.id)}
              >
                {t(mode.label)}
              </Button>
            ))}
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              MODES.find((mode) => mode.id === config?.mode)?.hint ??
                'Failed to load'
            )}
          </p>
          {isRoot ? (
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='w-fit'
              onClick={() => void rollback()}
            >
              {t('One-click back to official routing')}
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Routing preference')}</CardTitle>
          <CardDescription>
            {t(
              'Choose a named preference. Weights stay fixed in the backend.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          {schemesQuery.isError ? (
            <p className='text-destructive text-sm'>{t('Failed to load')}</p>
          ) : null}
          <div className='flex flex-wrap gap-2'>
            {PREFERENCES.map((pref) => (
              <Button
                key={pref.name}
                type='button'
                size='sm'
                variant={
                  config?.active_weight_scheme === pref.name
                    ? 'default'
                    : 'outline'
                }
                disabled={
                  !ready ||
                  !isRoot ||
                  schemesQuery.isError ||
                  putConfig.isPending
                }
                onClick={() => void usePreference(pref.name)}
              >
                {t(pref.label)}
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Fixed routing rules')}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Excluded channels are never selected again, even if that leaves no channel. Smart weights do not cross priority bands, and user billing does not follow the route score.'
            )}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Model preview')}</CardTitle>
          <CardDescription>
            {t(
              'Pick a model to see current share percentages. Does not send real upstream requests.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <div className='flex flex-wrap gap-2'>
            <Input
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder={t('Model name')}
              className='max-w-sm'
            />
            <Button type='button' size='sm' onClick={() => void runPreview()}>
              {t('Preview')}
            </Button>
          </div>
          {preview?.error ? (
            <p className='text-destructive text-sm'>{preview.error}</p>
          ) : null}
          {preview && !preview.error ? (
            <p className='text-muted-foreground text-sm'>
              {t('Dispatch')} #{preview.picked || '—'}
              {preview.would_pick
                ? ` · ${t('Shadow pick')} #${preview.would_pick}`
                : ''}
            </p>
          ) : null}
          {preview?.half_open ? (
            <p className='text-muted-foreground text-sm'>
              {t('Half-open')} #{preview.half_open.channel_id}
              {preview.half_open.share != null
                ? ` ${Math.round(preview.half_open.share * 1000) / 10}%`
                : ''}
            </p>
          ) : null}
          {preview && !preview.error ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Share')}</TableHead>
                  <TableHead>{t('Weight')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {preview.items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={3}>{t('No data yet')}</TableCell>
                  </TableRow>
                ) : (
                  preview.items.map((item) => (
                    <TableRow key={item.channel_id}>
                      <TableCell>
                        {item.channel_name || item.channel_id}
                      </TableCell>
                      <TableCell>{item.share_pct}</TableCell>
                      <TableCell>{item.route_weight}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
