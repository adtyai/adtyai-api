import { useQuery } from '@tanstack/react-query'
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

import { getCostBackfillStatus, startCostBackfill } from '../api'
import { routingErrorText } from '../lib/error-text'

const BACKFILL_QUERY_KEY = ['smart-routing', 'cost-backfill'] as const

/** Root-only: start a background cost backfill (202 + job_id) and poll its status. */
export function CostBackfillCard() {
  const { t } = useTranslation()
  // Track the job we started by its string id; ids are never coerced to
  // numbers (UnixNano exceeds Number.MAX_SAFE_INTEGER).
  const [jobId, setJobId] = useState<string | undefined>(undefined)
  const statusQuery = useQuery({
    queryKey: [...BACKFILL_QUERY_KEY, jobId ?? 'latest'],
    queryFn: () => getCostBackfillStatus(jobId),
    refetchInterval: (q) =>
      q.state.data?.data?.status === 'running' ? 2000 : false,
  })
  const envelope = statusQuery.data
  const notFound =
    envelope?.success === false && envelope.code === 'job_not_found'
  const job = envelope?.success ? envelope.data : null
  const running = job?.status === 'running'

  const start = async () => {
    const res = await startCostBackfill()
    if (!res.success) {
      toast.error(
        res.code === 'backfill_running'
          ? t('A backfill job is already running')
          : routingErrorText(t, res, t('Backfill failed to start'))
      )
      setJobId(undefined)
      void statusQuery.refetch()
      return
    }
    toast.success(t('Backfill started'))
    setJobId(res.data?.job_id != null ? String(res.data.job_id) : undefined)
  }

  let statusText = t('No backfill run yet')
  if (notFound) {
    statusText = t('Backfill job not found')
  } else if (job?.status === 'running') {
    statusText = t('Running…')
  } else if (job?.status === 'done') {
    statusText = t('Done: scanned {{scanned}}, inserted {{inserted}}', {
      scanned: job.scanned ?? 0,
      inserted: job.inserted ?? 0,
    })
  } else if (job?.status === 'failed') {
    statusText = `${t('Failed')}: ${job.error ?? ''}`
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Historical cost backfill')}</CardTitle>
        <CardDescription>
          {t('Rebuild cost ledger from usage logs in the background.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-wrap items-center gap-3'>
        <Button
          type='button'
          size='sm'
          disabled={running}
          onClick={() => void start()}
        >
          {t('Start backfill')}
        </Button>
        <span
          className={
            job?.status === 'failed' || notFound
              ? 'text-destructive text-sm'
              : 'text-muted-foreground text-sm'
          }
        >
          {job?.job_id != null ? `#${job.job_id} · ` : ''}
          {statusText}
        </span>
      </CardContent>
    </Card>
  )
}
