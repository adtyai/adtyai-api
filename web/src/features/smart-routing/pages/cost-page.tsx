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
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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

import { LiveEstimateCell, RatioRateCell } from '../components/cost-cells'
import { EmptyHint } from '../components/empty-hint'
import {
  useCostGuards,
  useCostRankings,
  usePrices,
  usePutPrice,
} from '../hooks/use-cost'
import type { ChannelPriceRow } from '../types'

export function CostPage() {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (s) => s.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const pricesQuery = usePrices()
  const guardsQuery = useCostGuards()
  const rankingsQuery = useCostRankings(isRoot)
  const putPrice = usePutPrice()
  const [editing, setEditing] = useState<ChannelPriceRow | null>(null)
  const [advancedOpen, setAdvancedOpen] = useState(false)

  const saveRow = async () => {
    if (!editing || !isRoot) return
    const res = await putPrice.mutateAsync({
      channel_id: editing.channel_id,
      recharge_credits_per_cny: editing.recharge_credits_per_cny,
      channel_ratio: editing.channel_ratio,
      absolute_usd_per_mtok: editing.absolute_enabled
        ? editing.absolute_usd_per_mtok
        : null,
      absolute_enabled: editing.absolute_enabled,
    })
    if (!res.success) {
      toast.error(res.message || t('Save failed'))
      return
    }
    toast.success(t('Cost settings saved'))
    setEditing(null)
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pb-4'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Cost entry (default: rate + multiplier)')}</CardTitle>
          <CardDescription>
            {t(
              'Fill recharge rate (1 CNY → N upstream USD credits) and channel multiplier (reuse NewAPI ratio). Absolute USD/1M is advanced only. Price score uses local ledger when samples exist.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          {(pricesQuery.data ?? []).length === 0 ? (
            <EmptyHint
              title={t('No channel cost rows')}
              description={t(
                'When /prices is online, edit rate and multiplier here.'
              )}
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Recharge rate')}</TableHead>
                  <TableHead>{t('Channel multiplier')}</TableHead>
                  <TableHead>{t('Multiplier × rate')}</TableHead>
                  <TableHead>{t('Last-30-min estimate')}</TableHead>
                  <TableHead>{t('Estimate share')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {(pricesQuery.data ?? []).map((row) => (
                  <TableRow key={row.channel_id}>
                    <TableCell>
                      {row.channel_name || `#${row.channel_id}`}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.recharge_credits_per_cny ?? '—'}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.channel_ratio ?? '—'}
                    </TableCell>
                    <TableCell className='tabular-nums text-xs'>
                      <RatioRateCell
                        missing={row.missing_cost}
                        value={row.cost_ratio_rate}
                      />
                    </TableCell>
                    <TableCell className='tabular-nums text-xs'>
                      <LiveEstimateCell value={row.cost_live_30m} />
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.estimate_sample_ratio != null
                        ? `${(row.estimate_sample_ratio * 100).toFixed(0)}%`
                        : '—'}
                    </TableCell>
                    <TableCell>
                      {isRoot ? (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => {
                            setEditing({ ...row })
                            setAdvancedOpen(Boolean(row.absolute_enabled))
                          }}
                        >
                          {t('Edit')}
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          {editing ? (
            <div className='bg-muted/30 flex flex-col gap-3 rounded-lg border p-4'>
              <p className='text-sm font-medium'>
                {t('Edit')}:{' '}
                {editing.channel_name || `#${editing.channel_id}`}
              </p>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='flex flex-col gap-1.5'>
                  <Label>{t('Recharge rate (1 CNY = N credits)')}</Label>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    value={editing.recharge_credits_per_cny ?? ''}
                    onChange={(e) => {
                      const n = Number(e.target.value)
                      setEditing({
                        ...editing,
                        recharge_credits_per_cny: Number.isFinite(n)
                          ? n
                          : null,
                      })
                    }}
                  />
                </div>
                <div className='flex flex-col gap-1.5'>
                  <Label>{t('Channel multiplier')}</Label>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    value={editing.channel_ratio ?? ''}
                    onChange={(e) => {
                      const n = Number(e.target.value)
                      setEditing({
                        ...editing,
                        channel_ratio: Number.isFinite(n) ? n : null,
                      })
                    }}
                  />
                  <p className='text-muted-foreground text-xs'>
                    {t('Reuses existing NewAPI channel ratio when possible.')}
                  </p>
                </div>
              </div>
              <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
                <CollapsibleTrigger className='text-sm underline'>
                  {t('Advanced: absolute USD per 1M tokens')}
                </CollapsibleTrigger>
                <CollapsibleContent className='mt-2 flex flex-col gap-1.5'>
                  <Input
                    type='number'
                    min={0}
                    step='0.01'
                    placeholder={t('USD / 1M tokens')}
                    value={editing.absolute_usd_per_mtok ?? ''}
                    onChange={(e) => {
                      const n = Number(e.target.value)
                      setEditing({
                        ...editing,
                        absolute_enabled: true,
                        absolute_usd_per_mtok: Number.isFinite(n) ? n : null,
                      })
                    }}
                  />
                </CollapsibleContent>
              </Collapsible>
              <div className='flex gap-2'>
                <Button onClick={() => void saveRow()} disabled={putPrice.isPending}>
                  {t('Save')}
                </Button>
                <Button variant='ghost' onClick={() => setEditing(null)}>
                  {t('Cancel')}
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Daily cost guardrails')}</CardTitle>
          <CardDescription>
            {t(
              'Channels exceeding ~3× 7-day daily average are excluded until cleared.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {(guardsQuery.data ?? []).length === 0 ? (
            <EmptyHint title={t('No guardrail events')} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Today')}</TableHead>
                  <TableHead>{t('7d avg')}</TableHead>
                  <TableHead>{t('Excluded')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(guardsQuery.data ?? []).map((g) => (
                  <TableRow key={g.channel_id}>
                    <TableCell>
                      {g.channel_name || `#${g.channel_id}`}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {(g.daily_cost_cny ?? 0).toFixed(2)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {(g.daily_avg_7d ?? 0).toFixed(2)}
                    </TableCell>
                    <TableCell>{g.excluded ? t('Yes') : t('No')}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {isRoot ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('Margin by channel (root)')}</CardTitle>
            <CardDescription>
              {t(
                'User billing is unchanged. This is a separate cost ledger for root only.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {(rankingsQuery.data ?? []).length === 0 ? (
              <EmptyHint title={t('No margin rankings')} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Channel')}</TableHead>
                    <TableHead>{t('Revenue')}</TableHead>
                    <TableHead>{t('Cost')}</TableHead>
                    <TableHead>{t('Margin')}</TableHead>
                    <TableHead>{t('Margin %')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(rankingsQuery.data ?? []).map((r) => (
                    <TableRow key={r.channel_id}>
                      <TableCell>
                        {r.channel_name || `#${r.channel_id}`}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {(r.revenue_cny ?? 0).toFixed(2)}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {(r.cost_cny ?? 0).toFixed(2)}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {(r.margin_cny ?? 0).toFixed(2)}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {r.margin_pct != null
                          ? `${r.margin_pct.toFixed(1)}%`
                          : '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
