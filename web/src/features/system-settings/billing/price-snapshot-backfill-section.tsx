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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsSection } from '../components/settings-section'

type BackfillSkip = {
  kind: string
  id: string
  reason: string
}

type BackfillReport = {
  dry_run: boolean
  filled_tasks: number
  filled_operations: number
  skipped: number
  already_complete: number
  amounts_preserved: boolean
  more_remain: boolean
  skipped_sample?: BackfillSkip[]
}

const backfillKey = ['adty-price-snapshot-backfill']

async function readBackfillReport(path: string, body?: unknown) {
  const response = body ? await api.post(path, body) : await api.get(path)
  const payload = response.data as {
    success?: boolean
    data?: BackfillReport
    message?: string
  }
  if (!payload.success || !payload.data) {
    throw new Error(payload.message || 'Failed to load data')
  }
  return payload.data
}

export function PriceSnapshotBackfillSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === ROLE.SUPER_ADMIN && !user.staff_site_id
  const [confirming, setConfirming] = useState(false)
  const preview = useQuery({
    queryKey: backfillKey,
    queryFn: () =>
      readBackfillReport('/api/adty/admin/billing/price-snapshot-backfill'),
    enabled: isRoot,
  })
  const apply = useMutation({
    mutationFn: () =>
      readBackfillReport('/api/adty/admin/billing/price-snapshot-backfill', {
        dry_run: false,
        limit: 200,
      }),
    onSuccess: async (report) => {
      setConfirming(false)
      if (report.amounts_preserved) {
        toast.success(
          t(
            'Filled {{tasks}} task snapshots and {{operations}} billing operations. Stored amounts were not changed.',
            {
              tasks: report.filled_tasks,
              operations: report.filled_operations,
            }
          )
        )
      } else {
        toast.error(
          t('The backfill stopped because a stored amount would have changed.')
        )
      }
      await queryClient.invalidateQueries({ queryKey: backfillKey })
    },
    onError: (error) => {
      toast.error(getServerErrorMessage(error, t('Failed to save')))
    },
  })
  if (!isRoot) return null
  const report = preview.data
  return (
    <SettingsSection title={t('Historical price snapshots')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Fill official decimal price snapshots from already-stored amounts and ratios. Historical reserved and actual amounts are not recalculated at current prices. Rows that cannot be filled safely are left unchanged and listed for review.'
        )}
      </p>
      {preview.isError && (
        <p className='text-destructive text-sm'>
          {getServerErrorMessage(preview.error, t('Failed to load data'))}
        </p>
      )}
      {report && (
        <dl className='grid gap-2 text-sm sm:grid-cols-2'>
          <div>
            <dt className='text-muted-foreground'>{t('Fillable task rows')}</dt>
            <dd>{report.filled_tasks}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('Fillable billing operations')}
            </dt>
            <dd>{report.filled_operations}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('Already complete')}</dt>
            <dd>{report.already_complete}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('Skipped, cannot fill safely')}
            </dt>
            <dd>{report.skipped}</dd>
          </div>
        </dl>
      )}
      {!!report?.skipped_sample?.length && (
        <ul className='text-muted-foreground list-disc pl-5 text-sm'>
          {report.skipped_sample.slice(0, 10).map((item) => (
            <li key={`${item.kind}-${item.id}`}>
              {item.kind} {item.id}: {item.reason}
            </li>
          ))}
        </ul>
      )}
      <div>
        <Button
          type='button'
          disabled={preview.isLoading || apply.isPending}
          onClick={() => setConfirming(true)}
        >
          {t('Backfill stored snapshots')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => !apply.isPending && setConfirming(open)}
        title={t('Backfill stored snapshots')}
        desc={t(
          'This copies already-stored amounts and ratios into snapshot fields. It does not reprice historical rows at current prices. Rows that cannot be filled safely will be skipped.'
        )}
        isLoading={apply.isPending}
        handleConfirm={() => apply.mutate()}
      />
    </SettingsSection>
  )
}
