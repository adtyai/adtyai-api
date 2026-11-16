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

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getPaymentScenes,
  paymentScenesKey,
  savePaymentScenes,
  type PaymentSceneConfig,
} from './api'
import { PaymentSceneEditor } from './scene-editor'

function SceneManagement(props: { userId: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [editing, setEditing] = useState<{
    config: PaymentSceneConfig
    index: number
  } | null>(null)
  const [removing, setRemoving] = useState<{
    config: PaymentSceneConfig
    index: number
  } | null>(null)
  const query = useQuery({
    queryKey: [...paymentScenesKey, props.userId, window.location.host],
    queryFn: getPaymentScenes,
  })
  const remove = useMutation({
    mutationFn: (target: { config: PaymentSceneConfig; index: number }) => {
      const user = useAuthStore.getState().auth.user
      if (user?.id !== props.userId || user.role < ROLE.SUPER_ADMIN) {
        throw new Error(t('Unauthorized, insufficient privileges'))
      }
      return savePaymentScenes({
        ...target.config,
        scenes: target.config.scenes.filter(
          (_, index) => index !== target.index
        ),
      })
    },
    onSuccess: () => {
      setRemoving(null)
      void client.invalidateQueries({ queryKey: paymentScenesKey })
      void client.invalidateQueries({ queryKey: ['adty-topup'] })
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Payment scenes')}</CardTitle>
      </CardHeader>
      <CardContent className='flex min-w-0 flex-col gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Control where each payment method can be used. Empty restrictions inherit the selected providers.'
          )}
        </p>
        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            size='sm'
            disabled={!query.data || query.isError}
            onClick={() =>
              query.data && setEditing({ config: query.data, index: -1 })
            }
          >
            {t('New payment scene')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('Refresh')}
          </Button>
        </div>
        {query.isLoading && <LoadingState />}
        {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
        {query.data?.scenes.length === 0 && (
          <EmptyState title={t('No payment scenes')} />
        )}
        {query.data?.scenes.map((scene, index) => (
          <div
            key={scene.key}
            className='flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-md border p-3'
          >
            <div className='min-w-0 flex-1 break-words'>
              <p className='font-medium'>{t(scene.name)}</p>
              <p className='text-muted-foreground text-sm'>{scene.key}</p>
            </div>
            <StatusBadge
              variant={scene.enabled ? 'success' : 'neutral'}
              copyable={false}
              label={t(scene.enabled ? 'Enabled' : 'Disabled')}
            />
            <div className='flex gap-2'>
              <Button
                type='button'
                size='sm'
                variant='outline'
                aria-label={t('Edit payment scene {{name}}', {
                  name: scene.name,
                })}
                onClick={() =>
                  query.data && setEditing({ config: query.data, index })
                }
              >
                {t('Edit')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='outline'
                aria-label={t('Delete payment scene {{name}}', {
                  name: scene.name,
                })}
                onClick={() =>
                  query.data && setRemoving({ config: query.data, index })
                }
              >
                {t('Delete')}
              </Button>
            </div>
          </div>
        ))}
        {editing && (
          <PaymentSceneEditor
            config={editing.config}
            index={editing.index}
            userId={props.userId}
            onClose={() => setEditing(null)}
          />
        )}
        <ConfirmDialog
          open={!!removing}
          onOpenChange={(open) =>
            !open && !remove.isPending && setRemoving(null)
          }
          title={t('Delete payment scene')}
          desc={t(
            'New orders will use these rules. Existing orders keep their original settings.'
          )}
          destructive
          isLoading={remove.isPending}
          handleConfirm={() => removing && remove.mutate(removing)}
        />
      </CardContent>
    </Card>
  )
}

export function PaymentSceneManager() {
  const user = useAuthStore((state) => state.auth.user)
  if (!user || user.role < ROLE.SUPER_ADMIN || user.staff_site_id) return null
  return <SceneManagement key={user.id} userId={user.id} />
}
