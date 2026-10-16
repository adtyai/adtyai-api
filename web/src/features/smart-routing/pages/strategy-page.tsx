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
import { Download, Upload } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
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

import {
  createWeightScheme,
  exportWeightScheme,
  importWeightScheme,
  previewWeightScheme,
  rollbackToOfficial,
  rollbackWeightScheme,
  simulateRouting,
} from '../api'
import { EmptyHint } from '../components/empty-hint'
import {
  DEFAULT_SCORE_WEIGHTS,
  WEIGHT_DIM_KEYS,
  WEIGHT_DIM_LABEL_KEYS,
} from '../constants'
import { previewIntentFor, validWeights } from '../lib/preview-intent'
import { fromBackendWeights } from '../lib/weight-map'
import {
  ROUTING_CONFIG_QUERY_KEY,
  usePutRoutingConfig,
  useRoutingConfig,
  useWeightHistory,
  useWeightSchemes,
  WEIGHT_HISTORY_QUERY_KEY,
  WEIGHT_SCHEMES_QUERY_KEY,
} from '../hooks/use-routing-config'
import type {
  ScoreWeights,
  SimulateResult,
  WeightHistoryEntry,
  WeightPreview,
} from '../types'

export function StrategyPage() {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (s) => s.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const configQuery = useRoutingConfig()
  const schemesQuery = useWeightSchemes()
  const historyQuery = useWeightHistory()
  const putConfig = usePutRoutingConfig()
  const qc = useQueryClient()

  const config = configQuery.data
  const [draftWeights, setDraftWeights] = useState<ScoreWeights | null>(null)
  const weights = draftWeights ?? config?.weights ?? DEFAULT_SCORE_WEIGHTS

  const [preview, setPreview] = useState<WeightPreview | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [, setPendingSchemeId] = useState<string | null>(null)
  const [pendingSchemeName, setPendingSchemeName] = useState<string | null>(
    null
  )
  const [pendingRollbackId, setPendingRollbackId] = useState<string | null>(
    null
  )
  const [pendingEnable, setPendingEnable] = useState<boolean | null>(null)
  const [schemeName, setSchemeName] = useState('')
  const [simulateModel, setSimulateModel] = useState('')
  const [simulateResult, setSimulateResult] = useState<SimulateResult | null>(
    null
  )

  const weightSum = useMemo(
    () =>
      WEIGHT_DIM_KEYS.reduce((acc, k) => acc + (Number(weights[k]) || 0), 0),
    [weights]
  )

  const saveSchemeMut = useMutation({
    mutationFn: async () => {
      if (!validWeights(weights)) {
        return {
          success: false,
          message: t('Weights must be non-negative and sum to more than 0'),
        }
      }
      return createWeightScheme({
        name: schemeName.trim() || t('Untitled scheme'),
        weights,
      })
    },
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Save failed'))
        return
      }
      toast.success(t('Scheme saved'))
      setSchemeName('')
      void qc.invalidateQueries({ queryKey: WEIGHT_SCHEMES_QUERY_KEY })
    },
  })

  const clearPendingPreview = () => {
    setPendingSchemeId(null)
    setPendingSchemeName(null)
    setPendingRollbackId(null)
  }

  const applyWeights = async (opts?: {
    schemeId?: string
    schemeName?: string
    nextWeights?: ScoreWeights
    kind?: string
  }) => {
    if (!isRoot) {
      toast.error(t('Only root can change routing strategy'))
      return
    }
    const next = opts?.nextWeights ?? weights
    if (!validWeights(next)) {
      toast.error(t('Weights must be non-negative and sum to more than 0'))
      return
    }
    if (opts?.nextWeights) {
      setDraftWeights({ ...opts.nextWeights })
    }
    setPendingRollbackId(null)
    setPendingSchemeId(opts?.schemeId ?? null)
    setPendingSchemeName(opts?.schemeName ?? null)
    // P1-3: the preview intent must fingerprint-match what PUT /config will
    // compute server-side (kind, scheme_id=0, scheme_name=new active scheme).
    const { kind, scheme_name: targetScheme } = previewIntentFor({
      activeScheme: config?.active_weight_scheme,
      currentWeights: config?.weights,
      nextWeights: next,
      schemeName: opts?.schemeName,
    })
    const result = await previewWeightScheme({
      weights: next,
      scheme_name: targetScheme,
      kind,
    })
    setPreview(result)
    setPreviewOpen(true)
  }

  const tokenOk = Boolean(preview?.preview_token)
  // P2-2: an empty preview (no scores yet) is still applicable.
  const previewReady = tokenOk
  const previewEmpty = Boolean(preview && preview.items.length === 0)

  const confirmApplyWeights = async () => {
    if (!previewReady || !preview?.preview_token) {
      toast.error(t('Preview required before apply'))
      return
    }
    const token = preview.preview_token

    if (pendingRollbackId) {
      const res = await rollbackWeightScheme({
        snapshot_id: pendingRollbackId,
        preview_token: token,
      })
      setPreviewOpen(false)
      clearPendingPreview()
      if (!res.success) {
        toast.error(res.message || t('Rollback failed'))
        return
      }
      toast.success(t('Rolled back'))
      setDraftWeights(null)
      void qc.invalidateQueries({ queryKey: WEIGHT_HISTORY_QUERY_KEY })
      void qc.invalidateQueries({ queryKey: ROUTING_CONFIG_QUERY_KEY })
      return
    }

    const schemeName =
      pendingSchemeName &&
      pendingSchemeName !== (config?.active_weight_scheme || undefined)
        ? pendingSchemeName
        : undefined
    const payload: Parameters<typeof putConfig.mutateAsync>[0] = {
      weights: draftWeights ?? weights,
      preview_token: token,
    }
    if (schemeName) {
      payload.active_weight_scheme = schemeName
    }
    const switched = Boolean(schemeName)
    const res = await putConfig.mutateAsync(payload)
    setPreviewOpen(false)
    clearPendingPreview()
    if (!res.success) {
      toast.error(res.message || t('Save failed'))
      return
    }
    toast.success(switched ? t('Switched scheme') : t('Weights applied'))
    setDraftWeights(null)
    void qc.invalidateQueries({ queryKey: WEIGHT_HISTORY_QUERY_KEY })
    void qc.invalidateQueries({ queryKey: ROUTING_CONFIG_QUERY_KEY })
  }

  const startRollbackPreview = async (h: WeightHistoryEntry) => {
    if (!isRoot) return
    let nextWeights = draftWeights ?? weights
    let schemeName: string | undefined
    if (h.snapshot) {
      try {
        const snap =
          typeof h.snapshot === 'string'
            ? (JSON.parse(h.snapshot) as Record<string, unknown>)
            : h.snapshot
        if (snap.weights) {
          nextWeights = fromBackendWeights(snap.weights)
        }
        if (typeof snap.active_weight_scheme === 'string') {
          schemeName = snap.active_weight_scheme
        }
      } catch {
        /* soft-fail: preview with current draft */
      }
    }
    if (h.scheme_name && !schemeName) schemeName = h.scheme_name
    setDraftWeights({ ...nextWeights })
    setPendingRollbackId(h.id)
    setPendingSchemeId(null)
    setPendingSchemeName(schemeName ?? null)
    const result = await previewWeightScheme({
      weights: nextWeights,
      scheme_name: schemeName,
      kind: 'put_config',
    })
    setPreview(result)
    setPreviewOpen(true)
  }

  const toggleEnabled = async (next: boolean) => {
    if (!isRoot) {
      toast.error(t('Only root can change routing strategy'))
      return
    }
    setPendingEnable(next)
  }

  const confirmToggle = async () => {
    if (pendingEnable == null) return
    const res = await putConfig.mutateAsync({ enabled: pendingEnable })
    setPendingEnable(null)
    if (!res.success) {
      toast.error(res.message || t('Save failed'))
      return
    }
    toast.success(
      pendingEnable
        ? t('Smart routing enabled')
        : t('Smart routing disabled — using official weights')
    )
    void qc.invalidateQueries({ queryKey: ROUTING_CONFIG_QUERY_KEY })
  }

  const runSimulate = async () => {
    if (!simulateModel.trim()) {
      toast.error(t('Enter a model name'))
      return
    }
    const result = await simulateRouting({ model: simulateModel.trim() })
    setSimulateResult(result)
  }

  const doExport = async (id: string) => {
    const data = await exportWeightScheme(id)
    if (!data) {
      toast.error(t('Export failed or backend unavailable'))
      return
    }
    const blob = new Blob([JSON.stringify(data, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `routing-scheme-${id}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  const doImport = async (file: File) => {
    try {
      const text = await file.text()
      const json = JSON.parse(text) as unknown
      const res = await importWeightScheme(json)
      if (!res.success) {
        toast.error(res.message || t('Import failed'))
        return
      }
      toast.success(t('Scheme imported'))
      void qc.invalidateQueries({ queryKey: WEIGHT_SCHEMES_QUERY_KEY })
    } catch {
      toast.error(t('Invalid JSON file'))
    }
  }

  const doRollbackOfficial = async () => {
    if (!isRoot) return
    const res = await rollbackToOfficial()
    if (!res.success) {
      toast.error(res.message || t('Rollback failed'))
      return
    }
    toast.success(t('Rolled back to official channel weights'))
    void qc.invalidateQueries({ queryKey: ROUTING_CONFIG_QUERY_KEY })
  }

  return (
    <div className='flex flex-col gap-4 overflow-auto pb-4'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Master switch')}</CardTitle>
          <CardDescription>
            {t(
              'When off, selection uses official NewAPI priority and weight only. No probe traffic is ever sent.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-wrap items-center gap-4'>
          <div className='flex items-center gap-2'>
            <Switch
              checked={Boolean(config?.enabled)}
              onCheckedChange={(v) => void toggleEnabled(v)}
              disabled={!isRoot || putConfig.isPending}
            />
            <span className='text-sm'>
              {config?.enabled ? t('Enabled') : t('Disabled')}
            </span>
          </div>
          {isRoot ? (
            <Button variant='outline' size='sm' onClick={() => void doRollbackOfficial()}>
              {t('One-click back to official routing')}
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Score weights')}</CardTitle>
          <CardDescription>
            {t(
              'Factory default Balanced is 30/25/20/15/10. You may save named schemes; do not invent "cheap/stable" product names.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-5'>
            {WEIGHT_DIM_KEYS.map((key) => (
              <div key={key} className='flex flex-col gap-1.5'>
                <Label htmlFor={`w-${key}`}>{t(WEIGHT_DIM_LABEL_KEYS[key])}</Label>
                <Input
                  id={`w-${key}`}
                  type='number'
                  min={0}
                  max={100}
                  value={weights[key]}
                  disabled={!isRoot}
                  onChange={(e) => {
                    const n = Number(e.target.value)
                    setDraftWeights({
                      ...weights,
                      [key]: Number.isFinite(n) ? n : 0,
                    })
                  }}
                />
              </div>
            ))}
          </div>
          <p className='text-muted-foreground text-xs'>
            {t('Sum')}: {weightSum}
            {weightSum !== 100
              ? ` · ${t('Will be normalized if not 100')}`
              : null}
          </p>
          <div className='flex flex-wrap items-end gap-2'>
            {isRoot ? (
              <>
                <Button
                  onClick={() =>
                    void applyWeights({ kind: 'put_config' })
                  }
                >
                  {t('Preview & apply')}
                </Button>
                <Input
                  className='max-w-xs'
                  placeholder={t('Scheme name')}
                  value={schemeName}
                  onChange={(e) => setSchemeName(e.target.value)}
                />
                <Button
                  variant='secondary'
                  disabled={saveSchemeMut.isPending}
                  onClick={() => saveSchemeMut.mutate()}
                >
                  {t('Save as named scheme')}
                </Button>
              </>
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('Read-only for non-root admins')}
              </p>
            )}
          </div>

          <div className='flex flex-col gap-2'>
            <Label>{t('Named schemes')}</Label>
            <div className='flex flex-wrap gap-2'>
              {(schemesQuery.data ?? []).map((s) => (
                <div
                  key={s.id}
                  className='bg-muted/40 flex items-center gap-2 rounded-md border px-2 py-1 text-sm'
                >
                  <button
                    type='button'
                    className='hover:underline'
                    disabled={!isRoot}
                    onClick={() => {
                      void applyWeights({
                        schemeId: s.id,
                        schemeName: s.name,
                        nextWeights: { ...s.weights },
                        kind: 'activate_scheme',
                      })
                    }}
                  >
                    {s.name}
                    {s.is_default ? ` (${t('default')})` : ''}
                  </button>
                  <Button
                    size='icon-sm'
                    variant='ghost'
                    aria-label={t('Export')}
                    onClick={() => void doExport(s.id)}
                  >
                    <Download className='size-3.5' />
                  </Button>
                </div>
              ))}
              {isRoot ? (
                <label className='inline-flex cursor-pointer items-center gap-1 text-sm'>
                  <Upload className='size-3.5' />
                  {t('Import JSON')}
                  <input
                    type='file'
                    accept='application/json,.json'
                    className='hidden'
                    onChange={(e) => {
                      const f = e.target.files?.[0]
                      if (f) void doImport(f)
                      e.target.value = ''
                    }}
                  />
                </label>
              ) : null}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Optional switches (default off)')}</CardTitle>
          <CardDescription>
            {t(
              'Session sticky, ε-greedy ≈1%, peer breaker, Telegram, region filter.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
          {(
            [
              ['session_sticky', 'Session sticky'],
              ['epsilon_greedy', 'ε-greedy exploration'],
              ['peer_breaker', 'Peer worse breaker'],
              ['telegram_alert', 'Telegram alerts'],
              ['region_filter', 'Region / compliance filter'],
            ] as const
          ).map(([field, labelKey]) => (
            <div key={field} className='flex items-center justify-between gap-2 rounded-md border px-3 py-2'>
              <span className='text-sm'>{t(labelKey)}</span>
              <Switch
                checked={Boolean(config?.[field])}
                disabled={!isRoot}
                onCheckedChange={(v) => {
                  void putConfig.mutateAsync({ [field]: v }).then((res) => {
                    if (!res.success) toast.error(res.message || t('Save failed'))
                  })
                }}
              />
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Simulate routing')}</CardTitle>
          <CardDescription>
            {t(
              'Pick a model to see current share percentages. Does not send real upstream requests.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <div className='flex flex-wrap gap-2'>
            <Input
              className='max-w-sm'
              placeholder={t('Model name')}
              value={simulateModel}
              onChange={(e) => setSimulateModel(e.target.value)}
            />
            <Button variant='secondary' onClick={() => void runSimulate()}>
              {t('Simulate')}
            </Button>
          </div>
          {simulateResult && simulateResult.items.length === 0 ? (
            <EmptyHint
              title={t('No candidates')}
              description={t('No share data returned for this model.')}
            />
          ) : null}
          {simulateResult && simulateResult.items.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Score')}</TableHead>
                  <TableHead>{t('Share')}</TableHead>
                  <TableHead>{t('Half-open')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {simulateResult.items.map((item) => (
                  <TableRow key={item.channel_id}>
                    <TableCell>
                      {item.channel_name || `#${item.channel_id}`}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {item.score ?? '—'}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {(item.share_pct ?? 0).toFixed(1)}%
                    </TableCell>
                    <TableCell>
                      {item.half_open ? t('Yes') : t('No')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Change history')}</CardTitle>
          <CardDescription>
            {t('Weight changes are audited; root can roll back to a snapshot.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {(historyQuery.data ?? []).length === 0 ? (
            <EmptyHint title={t('No history yet')} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Time')}</TableHead>
                  <TableHead>{t('By')}</TableHead>
                  <TableHead>{t('Summary')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {(historyQuery.data ?? []).map((h) => (
                  <TableRow key={h.id}>
                    <TableCell className='text-xs'>{h.at}</TableCell>
                    <TableCell>{h.by || '—'}</TableCell>
                    <TableCell>{h.summary || h.scheme_name || '—'}</TableCell>
                    <TableCell>
                      {isRoot ? (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => {
                            void startRollbackPreview(h)
                          }}
                        >
                          {t('Rollback')}
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Read-only parameters')}</CardTitle>
        </CardHeader>
        <CardContent className='text-muted-foreground grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3'>
          <div>
            {t('Hysteresis recover')}: {config?.hysteresis_recover ?? 65}
          </div>
          <div>
            {t('Breaker enter')}: {config?.breaker_enter ?? 40}
          </div>
          <div>
            {t('Floor total')}: ≤{config?.floor_total_pct ?? 30}%
          </div>
          <div>
            {t('Retry budget')}: ≤{config?.retry_budget_pct ?? 20}%
          </div>
          <div>
            {t('Half-open in-flight max')}:{' '}
            {config?.half_open_inflight_max ?? 2}
          </div>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={previewOpen}
        onOpenChange={(open) => {
          setPreviewOpen(open)
          if (!open) clearPendingPreview()
        }}
        title={
          pendingRollbackId
            ? t('Preview rollback')
            : t('Preview weight change')
        }
        desc={
          previewReady
            ? t(
                'Review route weight now → new below, then confirm. Changes take effect next scoring cycle.'
              )
            : t(
                'Backend preview unavailable, empty, or missing preview_token. Confirm is disabled until a valid preview is returned.'
              )
        }
        confirmText={t('Confirm apply')}
        disabled={!previewReady}
        handleConfirm={() => void confirmApplyWeights()}
      >
        {previewReady && previewEmpty ? (
          <p className='text-muted-foreground mt-3 text-sm'>
            {t('No scores yet; changes take effect once scoring starts.')}
          </p>
        ) : null}
        {preview && preview.items.length > 0 ? (
          <div className='mt-3 max-h-60 overflow-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('RW now')}</TableHead>
                  <TableHead>{t('RW new')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {preview.items.map((item) => (
                  <TableRow key={`${item.channel_id}-${item.model ?? ''}`}>
                    <TableCell>
                      {item.channel_name || `#${item.channel_id}`}
                      {item.model ? ` / ${item.model}` : ''}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {Number(item.share_now).toFixed(1)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {Number(item.share_new).toFixed(1)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : null}
      </ConfirmDialog>

      <ConfirmDialog
        open={pendingEnable != null}
        onOpenChange={(open) => {
          if (!open) setPendingEnable(null)
        }}
        title={
          pendingEnable
            ? t('Enable smart routing?')
            : t('Disable smart routing?')
        }
        desc={
          pendingEnable
            ? t('Traffic will follow scores within each priority layer.')
            : t('Selection falls back to official weights immediately.')
        }
        confirmText={t('Confirm')}
        handleConfirm={() => void confirmToggle()}
      />
    </div>
  )
}
