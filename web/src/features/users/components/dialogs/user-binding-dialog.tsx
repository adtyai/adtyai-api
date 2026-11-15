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
import { Mail, Link2, Unlink, Loader2, Eye, EyeOff } from 'lucide-react'
import { useState, useEffect, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getUser, adminClearUserBinding } from '../../api'
import type { User } from '../../types'
import { useUsers } from '../users-provider'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number | null
  onUnbindSuccess?: () => void
}

interface BindingItem {
  key: string
  label: string
  icon: React.ReactNode
  value: string
  isBound: boolean
}

const BUILTIN_BINDINGS: ReadonlyArray<{
  key: string
  field: string
  label: string
  icon: React.ReactNode
}> = [
  {
    key: 'email',
    field: 'email',
    label: 'Email',
    icon: <Mail className='h-4 w-4' />,
  },
]

export function UserBindingDialog(props: Props) {
  const { t } = useTranslation()
  const { requestVerification, verificationActive } = useUsers()
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(false)
  const [showBoundOnly, setShowBoundOnly] = useState(true)
  const [unbindTarget, setUnbindTarget] = useState<BindingItem | null>(null)
  const [unbinding, setUnbinding] = useState(false)

  const fetchData = useCallback(async () => {
    if (!props.userId) return
    setLoading(true)
    try {
      const userRes = await getUser(props.userId)
      requireServerSuccess(userRes)
      if (userRes.success && userRes.data) {
        setUser(userRes.data)
      }
    } catch (error) {
      handleServerError(error, t('Failed to load'))
    } finally {
      setLoading(false)
    }
  }, [props.userId, t])

  useEffect(() => {
    if (props.open && props.userId) {
      setShowBoundOnly(true)
      fetchData()
    } else {
      setUser(null)
    }
  }, [props.open, props.userId, fetchData])

  const allBindings = useMemo<BindingItem[]>(() => {
    return BUILTIN_BINDINGS.map((field) => {
      const value = user
        ? String((user as Record<string, unknown>)[field.field] || '')
        : ''
      return {
        key: field.key,
        label: field.label,
        icon: field.icon,
        value,
        isBound: Boolean(value),
      }
    })
  }, [user])

  const displayedBindings = showBoundOnly
    ? allBindings.filter((binding) => binding.isBound)
    : allBindings

  const boundCount = allBindings.filter((binding) => binding.isBound).length

  const handleUnbind = async () => {
    if (!unbindTarget || !props.userId) return
    setUnbinding(true)
    try {
      const userId = props.userId
      const proof = await requestVerification({
        scope: 'admin.user.binding.clear',
        title: t('Verify to unbind account'),
        description: t(
          'Confirm your identity before unbinding {{provider}} from this user.',
          { provider: unbindTarget.label }
        ),
        context: { user_id: userId, binding_type: unbindTarget.key },
      })
      if (!proof) return
      const res = await adminClearUserBinding(
        userId,
        unbindTarget.key,
        proof.proof_token
      )
      if (res?.success) {
        toast.success(
          t('Unbound {{provider}}', { provider: unbindTarget.label })
        )
        await fetchData()
        props.onUnbindSuccess?.()
      } else {
        handleServerError(res, t('Unbind failed'))
      }
    } catch (error) {
      handleServerError(AuthOperationError.from(error), t('Unbind failed'))
    } finally {
      setUnbinding(false)
      setUnbindTarget(null)
    }
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={
          <>
            <Link2 className='h-5 w-5' />
            {t('Account Binding Management')}
          </>
        }
        description={t('Manage account bindings for this user')}
        contentClassName='sm:max-w-lg'
        titleClassName='flex items-center gap-2'
        descriptionClassName='sr-only'
        contentHeight='auto'
        bodyClassName='space-y-4'
      >
        {loading ? (
          <div className='flex items-center justify-center py-8'>
            <Loader2 className='text-muted-foreground h-6 w-6 animate-spin' />
          </div>
        ) : (
          <div className='space-y-3'>
            <div className='flex items-center justify-between'>
              {user && (
                <p className='text-muted-foreground text-sm'>
                  {user.username} (ID: {user.id})
                </p>
              )}
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        variant='ghost'
                        size='sm'
                        className='h-7 gap-1.5 px-2 text-xs'
                        onClick={() => setShowBoundOnly((v) => !v)}
                      />
                    }
                  >
                    {showBoundOnly ? (
                      <Eye className='h-3.5 w-3.5' />
                    ) : (
                      <EyeOff className='h-3.5 w-3.5' />
                    )}
                    {showBoundOnly ? t('Show All') : t('Bound Only')}
                  </TooltipTrigger>
                  <TooltipContent>
                    {showBoundOnly
                      ? t('Show all providers including unbound')
                      : t('Show only bound providers')}
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>

            <Separator />

            <ScrollArea className='max-h-[50vh]'>
              {displayedBindings.length === 0 ? (
                <p className='text-muted-foreground py-4 text-center text-sm'>
                  {showBoundOnly
                    ? t('This user has no bindings')
                    : t('No providers available')}
                </p>
              ) : (
                <div className='grid grid-cols-1 gap-2 pr-3 lg:grid-cols-2'>
                  {displayedBindings.map((binding) => (
                    <div
                      key={binding.key}
                      className={`flex items-center justify-between rounded-md border px-3 py-2.5 ${
                        !binding.isBound ? 'opacity-50' : ''
                      }`}
                    >
                      <div className='flex min-w-0 items-center gap-2.5'>
                        <div className='text-muted-foreground shrink-0'>
                          {binding.icon}
                        </div>
                        <div className='min-w-0'>
                          <div className='flex items-center gap-1.5'>
                            <span className='text-sm font-medium'>
                              {binding.label}
                            </span>
                          </div>
                          <p className='text-muted-foreground max-w-[140px] truncate text-xs'>
                            {binding.isBound ? binding.value : t('Not bound')}
                          </p>
                        </div>
                      </div>
                      {binding.isBound && (
                        <Button
                          variant='ghost'
                          size='sm'
                          className='text-destructive hover:text-destructive h-7 w-7 shrink-0 p-0'
                          onClick={() => setUnbindTarget(binding)}
                        >
                          <Unlink className='h-3.5 w-3.5' />
                        </Button>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </ScrollArea>

            <p className='text-muted-foreground text-xs'>
              {t('Bound')}: {boundCount} / {allBindings.length}
            </p>
          </div>
        )}
      </Dialog>

      <ConfirmDialog
        open={!!unbindTarget && !verificationActive}
        onOpenChange={(open) => !open && setUnbindTarget(null)}
        title={t('Confirm Unbind')}
        desc={t(
          'Are you sure you want to unbind {{provider}} for this user? The user will no longer be able to log in via this method.',
          {
            provider: unbindTarget?.label || '',
          }
        )}
        confirmText={t('Confirm Unbind')}
        destructive
        handleConfirm={handleUnbind}
        isLoading={unbinding}
      />
    </>
  )
}
