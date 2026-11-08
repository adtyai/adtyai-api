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
import { Mail } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import { handleServerError } from '@/lib/handle-server-error'

import { getMarketingSelf, marketingKeys, saveMarketingSelf } from '../api'

/**
 * Profile card for product-update emails (adty task 12.1). Turning it off
 * only stops marketing; verification and top-up emails still arrive.
 */
export function MarketingPreferencesCard() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: marketingKeys.self,
    queryFn: getMarketingSelf,
  })
  const save = useMutation({
    mutationFn: (consent: boolean) =>
      saveMarketingSelf({
        consent,
        lang: i18n.resolvedLanguage || i18n.language,
      }),
    onSuccess: (next) => {
      queryClient.setQueryData(marketingKeys.self, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })
  const data = query.data
  if (query.isError) return <ErrorState onRetry={() => void query.refetch()} />
  if (!data?.enabled) return null

  return (
    <TitledCard
      title={t('Email subscriptions')}
      description={t(
        'Account emails such as verification codes and top-up receipts are always sent.'
      )}
      icon={<Mail className='h-4 w-4' />}
      iconTone='chart-2'
      disableHoverEffect
    >
      <div className='flex items-center justify-between gap-4'>
        <div className='space-y-1'>
          <Label htmlFor='marketing-self-consent'>
            {t('Product updates and offers')}
          </Label>
          {data.blocked && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Emails to this address bounced or were reported as spam, so we no longer send offers to it.'
              )}
            </p>
          )}
        </div>
        <Switch
          id='marketing-self-consent'
          checked={data.consent}
          disabled={save.isPending}
          onCheckedChange={(value) => save.mutate(value)}
        />
      </div>
    </TitledCard>
  )
}
