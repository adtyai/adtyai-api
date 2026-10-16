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

import { Info, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { ConfirmDeleteDialog } from './components/confirm-delete-dialog'
import { RuleFormDialog } from './components/rule-form-dialog'
import { useEnterprisePricing } from './hooks/use-enterprise-pricing'
import { formatDiscount, scopeLabel } from './lib/discount'
import type { EnterprisePriceRule } from './types'

export function EnterprisePricingSettings() {
  const { t } = useTranslation()
  const [userFilterDraft, setUserFilterDraft] = useState('')
  const [userFilter, setUserFilter] = useState<number | undefined>(undefined)
  const [page, setPage] = useState(0)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<EnterprisePriceRule | null>(null)
  const [deleteRow, setDeleteRow] = useState<EnterprisePriceRule | null>(null)

  const list = useEnterprisePricing(userFilter, page)
  const rows = list.data?.rows ?? []
  const total = list.data?.total ?? 0
  const pageSize = 50
  const isForbidden = (list.error as { status?: number } | null)?.status === 403
  const wildcard = t('All (wildcard)')

  function applyUserFilter() {
    const raw = userFilterDraft.trim()
    if (!raw) {
      setUserFilter(undefined)
      setPage(0)
      return
    }
    const n = Number(raw)
    if (!Number.isInteger(n) || n <= 0) return
    setUserFilter(n)
    setPage(0)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Enterprise pricing')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='flex items-center gap-2'>
          <Input
            className='h-8 w-36 font-mono text-xs'
            placeholder={t('Filter user ID')}
            value={userFilterDraft}
            onChange={(e) => setUserFilterDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') applyUserFilter()
            }}
          />
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8'
            onClick={applyUserFilter}
          >
            {t('Filter')}
          </Button>
        </div>
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
          {t('Create rule')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Enterprise agreement pricing. Match priority: group+model > model only > group only > all. Applied as group ratio × discount. Create / update / delete require a second confirmation.'
            )}
          </p>

          <div className='grid gap-3'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Empty group_code / model_name = wildcard. Discount 0.1–1 (show as %). Expires_at is RFC3339; clear on edit with empty value.'
                )}
              </div>
            </div>
          </div>

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'You do not have permission to manage enterprise pricing.'
              )}
            />
          ) : null}

          {list.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}

          {list.isError && !isForbidden && !list.data ? (
            <ErrorState
              title={t('Failed to load enterprise pricing')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
              <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                <div className='text-[15px] font-semibold'>
                  {t('Pricing rules')}
                </div>
                <div className='text-muted-foreground flex items-center gap-3 text-xs'>
                  <span>
                    {total} {t('items')}
                  </span>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-7 px-2 text-xs'
                    disabled={page <= 0}
                    onClick={() => setPage((p) => Math.max(0, p - 1))}
                  >
                    {t('Prev')}
                  </Button>
                  <span>
                    {t('Page')} {page + 1}
                  </span>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-7 px-2 text-xs'
                    disabled={(page + 1) * pageSize >= total}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    {t('Next')}
                  </Button>
                </div>
              </div>
              <Table>
                <TableHeader>
                  <TableRow className='bg-muted/40 hover:bg-muted/40'>
                    <TableHead>ID</TableHead>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Group')}</TableHead>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Discount')}</TableHead>
                    <TableHead>{t('Expires at')}</TableHead>
                    <TableHead>{t('Updated at')}</TableHead>
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
                        {t('No enterprise pricing rules')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const expired =
                        row.expires_at != null &&
                        new Date(row.expires_at).getTime() <= Date.now()
                      return (
                        <TableRow key={row.id}>
                          <TableCell>
                            <code className='text-xs'>{row.id}</code>
                          </TableCell>
                          <TableCell>{row.user_id}</TableCell>
                          <TableCell>
                            <Badge variant='secondary' className='font-mono'>
                              {scopeLabel(row.group_code, wildcard)}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <Badge variant='secondary' className='font-mono'>
                              {scopeLabel(row.model_name, wildcard)}
                            </Badge>
                          </TableCell>
                          <TableCell className='font-mono text-xs'>
                            {formatDiscount(row.discount)}
                          </TableCell>
                          <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                            {row.expires_at ? (
                              <span
                                className={
                                  expired
                                    ? 'font-medium text-rose-600 dark:text-rose-300'
                                    : ''
                                }
                              >
                                {new Date(row.expires_at).toLocaleString()}
                                {expired ? ` · ${t('Expired')}` : ''}
                              </span>
                            ) : (
                              t('Never')
                            )}
                          </TableCell>
                          <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                            {row.updated_at
                              ? new Date(row.updated_at).toLocaleString()
                              : '-'}
                          </TableCell>
                          <TableCell>
                            <div className='flex flex-wrap items-center gap-1.5'>
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
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                className='h-7 px-2.5 text-xs text-rose-700 dark:text-rose-300'
                                onClick={() => setDeleteRow(row)}
                              >
                                {t('Delete')}
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </section>
          ) : null}
        </div>
      </SectionPageLayout.Content>

      <RuleFormDialog
        open={formOpen}
        editing={editing}
        onOpenChange={(open) => {
          setFormOpen(open)
          if (!open) setEditing(null)
        }}
      />
      <ConfirmDeleteDialog
        row={deleteRow}
        open={Boolean(deleteRow)}
        onOpenChange={(open) => {
          if (!open) setDeleteRow(null)
        }}
      />
    </SectionPageLayout>
  )
}

export default EnterprisePricingSettings
