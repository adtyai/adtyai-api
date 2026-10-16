import { useQuery } from '@tanstack/react-query'
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
  const statusQuery = useQuery({
    queryKey: BACKFILL_QUERY_KEY,
    queryFn: getCostBackfillStatus,
    refetchInterval: (q) => (q.state.data?.status === 'running' ? 2000 : false),
  })
  const job = statusQuery.data
  const running = job?.status === 'running'

  const start = async () => {
    const res = await startCostBackfill()
    if (!res.success) {
      toast.error(
        res.code === 'backfill_running'
          ? t('A backfill job is already running')
          : routingErrorText(t, res, t('Backfill failed to start'))
      )
      void statusQuery.refetch()
      return
    }
    toast.success(t('Backfill started'))
    void statusQuery.refetch()
  }

  let statusText = t('No backfill run yet')
  if (job?.status === 'running') {
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
            job?.status === 'failed'
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
