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
import { Plus, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
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

import { SettingsSection } from '../components/settings-section'
import { ConfirmEpayActionDialog } from './components/confirm-epay-action-dialog'
import { InstanceFormDialog } from './components/instance-form-dialog'
import { useEpayInstances } from './hooks/use-epay-instances'
import {
  formatDailyLimit,
  formatSecretFingerprint,
  primaryInstance,
} from './lib/display'
import type { EpayInstanceView } from './types'

type Action = 'set-primary' | 'disable' | 'enable'

/**
 * Embeddable epay-instances block for Payment Gateway billing section.
 * Keeps list/create/edit/primary/enable/disable; drops page chrome and long copy.
 */
export function EpayInstancesSection() {
  const { t } = useTranslation()
  const list = useEpayInstances()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<EpayInstanceView | null>(null)
  const [actionRow, setActionRow] = useState<EpayInstanceView | null>(null)
  const [action, setAction] = useState<Action | null>(null)

  const data = list.data
  const rows = data ?? []
  const primary = useMemo(() => primaryInstance(data ?? []), [data])
  const isForbidden =
    (list.error as { status?: number } | null)?.status === 403

  return (
    <SettingsSection title={t('Epay instances')}>
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='text-muted-foreground text-sm'>
            {primary ? (
              <span>
                {t('Current primary')}:{' '}
                <span className='text-foreground font-medium'>
                  {primary.name}
                </span>
                <span className='ml-2 font-mono text-[11px] opacity-80'>
                  #{primary.id} · {t('Failures')}: {primary.failure_streak}
                </span>
              </span>
            ) : (
              t('No usable primary — Alipay/WeChat under maintenance')
            )}
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='h-8'
              onClick={() => void list.refetch()}
            >
              <RefreshCw className='size-3.5' />
              {t('Refresh')}
            </Button>
            <Button
              type='button'
              size='sm'
              className='h-8'
              onClick={() => {
                setEditing(null)
                setFormOpen(true)
              }}
            >
              <Plus className='size-3.5' />
              {t('Create instance')}
            </Button>
          </div>
        </div>

        {isForbidden ? (
          <ErrorState
            title={t('Insufficient permissions')}
            description={t(
              'You do not have permission to manage epay instances.'
            )}
          />
        ) : null}

        {list.isLoading ? (
          <div className='text-muted-foreground flex min-h-24 items-center justify-center text-sm'>
            {t('Loading...')}
          </div>
        ) : null}

        {list.isError && !isForbidden && !list.data ? (
          <ErrorState
            title={t('Failed to load epay instances')}
            onRetry={() => void list.refetch()}
          />
        ) : null}

        {!list.isLoading && list.data && !isForbidden ? (
          <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
            <Table>
              <TableHeader>
                <TableRow className='bg-muted/40 hover:bg-muted/40'>
                  <TableHead>ID</TableHead>
                  <TableHead>
                    {t('Name')} / {t('Gateway')}
                  </TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Daily limit')}</TableHead>
                  <TableHead>{t('Failures')}</TableHead>
                  <TableHead>{t('Secret')}</TableHead>
                  <TableHead>{t('Demoted')}</TableHead>
                  <TableHead className='w-[1%]' />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={8}
                      className='text-muted-foreground h-24 text-center text-sm'
                    >
                      {t('No epay instances')}
                    </TableCell>
                  </TableRow>
                ) : (
                  rows.map((row) => (
                    <TableRow key={row.id}>
                      <TableCell>
                        <code className='text-xs'>{row.id}</code>
                      </TableCell>
                      <TableCell>
                        <div className='font-medium'>{row.name}</div>
                        <div className='text-muted-foreground font-mono text-[11px]'>
                          {row.pay_address} · PID {row.partner_id}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap gap-1'>
                          {row.is_primary ? (
                            <Badge className='bg-indigo-50 text-indigo-700 ring-1 ring-indigo-200 dark:bg-indigo-950/40 dark:text-indigo-200'>
                              {t('Primary')}
                            </Badge>
                          ) : null}
                          <Badge
                            variant='secondary'
                            className={
                              row.status === 'enabled'
                                ? ''
                                : 'bg-slate-100 text-slate-600'
                            }
                          >
                            {row.status === 'enabled'
                              ? t('Enabled')
                              : t('Disabled')}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className='font-mono text-xs'>
                        {formatDailyLimit(row.daily_limit)}
                      </TableCell>
                      <TableCell>
                        <span
                          className={
                            row.failure_streak > 0
                              ? 'font-mono text-xs font-semibold text-rose-600'
                              : 'font-mono text-xs'
                          }
                        >
                          {row.failure_streak}
                        </span>
                      </TableCell>
                      <TableCell>
                        <div className='text-xs'>
                          {row.secret_set ? (
                            <span className='text-emerald-700 dark:text-emerald-300'>
                              {t('Secret set')}
                            </span>
                          ) : (
                            <span className='text-rose-600'>
                              {t('Secret missing')}
                            </span>
                          )}
                        </div>
                        <div className='text-muted-foreground font-mono text-[11px]'>
                          {formatSecretFingerprint(row.secret_fingerprint)}
                        </div>
                      </TableCell>
                      <TableCell>
                        <span
                          className='text-muted-foreground line-clamp-2 text-xs'
                          data-table-text='secondary'
                        >
                          {row.demoted_reason
                            ? row.demoted_reason +
                              (row.demoted_at
                                ? ' · ' +
                                  new Date(row.demoted_at).toLocaleString()
                                : '')
                            : '-'}
                        </span>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap items-center gap-1.5'>
                          {!row.is_primary && row.status === 'enabled' ? (
                            <>
                              <Button
                                type='button'
                                variant='outline'
                                size='sm'
                                className='h-7 px-2.5 text-xs'
                                onClick={() => {
                                  setActionRow(row)
                                  setAction('set-primary')
                                }}
                              >
                                {t('Set as primary')}
                              </Button>
                            </>
                          ) : null}
                          {row.status === 'enabled' ? (
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              className='h-7 px-2.5 text-xs'
                              onClick={() => {
                                setActionRow(row)
                                setAction('disable')
                              }}
                            >
                              {t('Disable')}
                            </Button>
                          ) : (
                            <>
                              <Button
                                type='button'
                                variant='outline'
                                size='sm'
                                className='h-7 px-2.5 text-xs'
                                onClick={() => {
                                  setActionRow(row)
                                  setAction('enable')
                                }}
                              >
                                {t('Enable')}
                              </Button>
                            </>
                          )}
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            className='h-7 px-2.5 text-xs'
                            onClick={() => {
                              setEditing(row)
                              setFormOpen(true)
                            }}
                          >
                            {t('Edit')}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </section>
        ) : null}
      </div>

      <InstanceFormDialog
        open={formOpen}
        editing={editing}
        onOpenChange={(open) => {
          setFormOpen(open)
          if (!open) setEditing(null)
        }}
      />
      <ConfirmEpayActionDialog
        row={actionRow}
        action={action}
        open={Boolean(actionRow && action)}
        onOpenChange={(open) => {
          if (!open) {
            setActionRow(null)
            setAction(null)
          }
        }}
      />
    </SettingsSection>
  )
}
