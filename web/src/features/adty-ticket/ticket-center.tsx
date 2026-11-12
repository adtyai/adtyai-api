import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

type Ticket = {
  id: number
  title: string
  status: string
}

type TicketMessage = {
  id: number
  visibility: string
  body: string
}

type TicketDetail = {
  ticket: Ticket
  messages: TicketMessage[]
  can_note: boolean
}

export function TicketCenter() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const list = useQuery({
    queryKey: ['adty-tickets'],
    queryFn: async () => {
      const response = await api.get('/api/adty/tickets')
      const body = response.data as { success?: boolean; data?: { tickets?: Ticket[] }; message?: string }
      if (!body.success) throw new Error(body.message || 'Failed to load data')
      return body.data?.tickets ?? []
    },
  })
  const [selected, setSelected] = useState<number | null>(null)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [reply, setReply] = useState('')
  const [note, setNote] = useState('')
  const [related, setRelated] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  useEffect(() => {
    const hash = window.location.hash.replace('#', '')
    if (hash.startsWith('ticket-')) setRelated(hash.slice('ticket-'.length))
  }, [])

  const detail = useQuery({
    queryKey: ['adty-tickets', selected],
    enabled: selected !== null,
    queryFn: async () => {
      const response = await api.get(`/api/adty/tickets/${selected}`)
      const payload = response.data as { success?: boolean; data?: TicketDetail; message?: string }
      if (!payload.success || !payload.data) throw new Error(payload.message || 'Failed to load data')
      return payload.data
    },
  })

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['adty-tickets'] })
  }

  const create = async () => {
    setPending(true)
    setError('')
    try {
      const response = await api.post('/api/adty/tickets', {
        type: 'other',
        title: title.trim(),
        body: body.trim(),
        priority: 'normal',
        related_kind: related ? 'topup' : '',
        related_ref: related,
      })
      const payload = response.data as { success?: boolean; message?: string; data?: Ticket }
      if (!payload.success || !payload.data) throw new Error(payload.message || 'Failed to save')
      setTitle('')
      setBody('')
      setSelected(payload.data.id)
      await refresh()
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  return (
    <Card id='tickets'>
      <CardContent className='flex flex-col gap-4 py-4'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Tickets')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Images only. Hide API keys, passwords, and payment details.')}
          </p>
        </div>
        {list.isLoading && <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>}
        {list.isError && (
          <Button type='button' variant='outline' onClick={() => void list.refetch()}>
            {t('Retry')}
          </Button>
        )}
        {list.data && list.data.length === 0 && <p className='text-muted-foreground text-sm'>{t('No tickets')}</p>}
        <div className='flex flex-col gap-2'>
          {(list.data ?? []).map((ticket) => (
            <button
              key={ticket.id}
              type='button'
              className='hover:bg-muted flex items-center justify-between gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors'
              onClick={() => setSelected(ticket.id)}
            >
              <span className='min-w-0 truncate'>{ticket.title}</span>
              <span className='text-muted-foreground shrink-0'>{ticket.status}</span>
            </button>
          ))}
        </div>
        {detail.data && (
          <div className='flex flex-col gap-3'>
            <h3 className='text-sm font-medium'>{detail.data.ticket.title}</h3>
            {detail.data.messages.map((message) => (
              <p key={message.id} className='text-sm whitespace-pre-wrap'>
                {message.visibility === 'internal' ? `${t('Internal note')}: ` : ''}
                {message.body}
              </p>
            ))}
            <Textarea value={reply} onChange={(event) => setReply(event.target.value)} />
            <Button
              type='button'
              disabled={pending || reply.trim() === ''}
              onClick={() =>
                void api.post(`/api/adty/tickets/${selected}/replies`, { body: reply.trim() }).then(() => {
                  setReply('')
                  return refresh()
                })
              }
            >
              {t('Reply')}
            </Button>
            {detail.data.can_note && (
              <>
                <Textarea value={note} onChange={(event) => setNote(event.target.value)} />
                <Button
                  type='button'
                  variant='outline'
                  disabled={pending || note.trim() === ''}
                  onClick={() =>
                    void api.post(`/api/adty/tickets/${selected}/notes`, { body: note.trim() }).then(() => {
                      setNote('')
                      return refresh()
                    })
                  }
                >
                  {t('Internal note')}
                </Button>
              </>
            )}
            <Label htmlFor='ticket-image'>{t('Image')}</Label>
            <Input
              id='ticket-image'
              type='file'
              accept='image/png,image/jpeg,image/gif,image/webp'
              onChange={(event) => {
                const file = event.target.files?.[0]
                if (!file || selected === null) return
                const reader = new FileReader()
                reader.onload = () => {
                  const value = String(reader.result || '')
                  void api
                    .post(`/api/adty/tickets/${selected}/images`, { data: value.slice(value.indexOf(',') + 1) })
                    .then(() => refresh())
                    .catch((cause) => setError(getServerErrorMessage(cause, t('Only images are accepted'))))
                }
                reader.readAsDataURL(file)
              }}
            />
          </div>
        )}
        <Label htmlFor='ticket-title'>{t('Title')}</Label>
        <Input id='ticket-title' value={title} onChange={(event) => setTitle(event.target.value)} />
        <Label htmlFor='ticket-body'>{t('Description')}</Label>
        <Textarea id='ticket-body' value={body} onChange={(event) => setBody(event.target.value)} />
        {related && <p className='text-muted-foreground text-sm tabular-nums'>{related}</p>}
        <Button type='button' disabled={pending || title.trim() === '' || body.trim() === ''} onClick={() => void create()}>
          {t('New ticket')}
        </Button>
        {error && <p className='text-destructive text-sm'>{error}</p>}
      </CardContent>
    </Card>
  )
}
