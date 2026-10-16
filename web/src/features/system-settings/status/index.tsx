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

import { Info, RefreshCw } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useHealthSummary, useRefreshHealth } from './hooks/use-health'
import {
  formatLatencyMs,
  formatProbeStatusLabel,
  probeErrorLabelKey,
  formatProbeTime,
  getProbeStatus,
  resolveProbeDisplay,
  sortProbesByTarget,
  summarizeProbeHealth,
  type ProbeStatus,
} from './lib/format'

function probeBadgeVariant(
  status: ProbeStatus
): 'secondary' | 'destructive' | 'outline' {
  if (status === 'ok') return 'secondary'
  if (status === 'skipped') return 'outline'
  return 'destructive'
}

function probeLabelText(
  t: (key: string, opts?: Record<string, string>) => string,
  targetType: string,
  targetId: string
): string {
  const d = resolveProbeDisplay(targetType, targetId)
  if (d.kind === 'raw') return d.name
  if (d.kind === 'withId') return t(d.key, { id: d.id })
  return t(d.key)
}

export function HealthStatusSettings() {
  const { t } = useTranslation()
  const healthQuery = useHealthSummary()
  const refreshMutation = useRefreshHealth()

  const rows = useMemo(
    () => sortProbesByTarget(healthQuery.data?.probes ?? []),
    [healthQuery.data?.probes]
  )

  const summary = useMemo(() => summarizeProbeHealth(rows), [rows])

  const isForbidden =
    (healthQuery.error as { status?: number } | null)?.status === 403

  async function onRefresh() {
    try {
      const data = await refreshMutation.mutateAsync()
      toast.success(
        data.refreshed
          ? t('Probes refreshed')
          : t('Probe summary loaded')
      )
    } catch (err) {
      if ((err as { status?: number })?.status === 403) {
        toast.error(
          t('Admin permission is required to view service status.')
        )
        return
      }
      toast.error(getServerErrorMessage(err, t('Refresh probes failed')))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Service status')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void healthQuery.refetch()}
          disabled={healthQuery.isFetching || refreshMutation.isPending}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          size='sm'
          className='h-8'
          onClick={() => void onRefresh()}
          disabled={refreshMutation.isPending || healthQuery.isLoading}
        >
          <RefreshCw
            className={`size-3.5${refreshMutation.isPending ? ' animate-spin' : ''}`}
          />
          {refreshMutation.isPending
            ? t('Refreshing probes...')
            : t('Refresh probes')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Latest health probes for postgres/redis, epay, bepusdt and chatwoot. Refresh probes runs a configured round and updates this table. Empty configs show as Not configured and do not count as unhealthy.'
            )}
          </p>

          <div className='grid gap-3'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'GET loads the summary on enter; Refresh probes calls POST /api/admin/health/refresh (no confirm, same as other admin tools).'
                )}
              </div>
            </div>
          </div>

          {healthQuery.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading settings...')}
            </div>
          ) : null}

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'Admin permission is required to view service status.'
              )}
            />
          ) : null}

          {healthQuery.isError && !healthQuery.data && !isForbidden ? (
            <ErrorState
              title={t('Failed to load settings')}
              onRetry={() => void healthQuery.refetch()}
            />
          ) : null}

          {healthQuery.data ? (
            <div className='bg-card space-y-3 rounded-xl border p-5 shadow-xs'>
              <div className='flex flex-wrap items-center gap-2 text-xs'>
                {healthQuery.data.refreshed ? (
                  <Badge variant='secondary'>{t('Refreshed this load')}</Badge>
                ) : (
                  <Badge variant='outline'>{t('Cached summary')}</Badge>
                )}
                {summary.overall === 'unhealthy' && (
                  <Badge variant='destructive'>
                    {t('Unhealthy')}: {summary.unhealthy}
                  </Badge>
                )}
                {summary.overall === 'healthy' && (
                  <Badge variant='secondary'>{t('All healthy')}</Badge>
                )}
                <span className='text-muted-foreground'>
                  {t('Probe rows')}: {rows.length}
                </span>
                {summary.skipped > 0 ? (
                  <span className='text-muted-foreground'>
                    {t('Not configured')}: {summary.skipped}
                  </span>
                ) : null}
              </div>

              <div className='overflow-hidden rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Probe')}</TableHead>
                      <TableHead>{t('Target')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Latency')}</TableHead>
                      <TableHead>{t('Error')}</TableHead>
                      <TableHead>{t('Checked at')}</TableHead>
                      <TableHead>{t('Last failure')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-muted-foreground text-sm'
                        >
                          {t('No probe records yet')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      rows.map((row) => {
                        const status = getProbeStatus(row)
                        const statusLabel = formatProbeStatusLabel(status)
                        const probeErrorText = (
                          s: typeof status,
                          error: string | null | undefined
                        ): string => {
                          if (s === 'skipped' || !error) return '—'
                          const k = probeErrorLabelKey(error)
                          return k ? t(k) : error
                        }
                        return (
                          <TableRow
                            key={`${row.target_type}:${row.target_id}:${row.checked_at}`}
                          >
                            <TableCell className='text-xs font-medium'>
                              {probeLabelText(t, row.target_type, row.target_id)}
                            </TableCell>
                            <TableCell className='font-mono text-xs'>
                              {row.target_id || '—'}
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant={probeBadgeVariant(status)}
                                className={
                                  status === 'skipped'
                                    ? 'text-muted-foreground'
                                    : undefined
                                }
                              >
                                {t(statusLabel)}
                              </Badge>
                            </TableCell>
                            <TableCell className='font-mono text-xs'>
                              {status === 'skipped'
                                ? '—'
                                : formatLatencyMs(row.latency_ms)}
                            </TableCell>
                            <TableCell className='max-w-[240px] truncate font-mono text-xs'>
                              {probeErrorText(status, row.error)}
                            </TableCell>
                            <TableCell className='font-mono text-xs whitespace-nowrap'>
                              {formatProbeTime(row.checked_at)}
                            </TableCell>
                            <TableCell className='font-mono text-xs whitespace-nowrap'>
                              {status === 'skipped'
                                ? '—'
                                : formatProbeTime(row.last_failure_at)}
                            </TableCell>
                          </TableRow>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
