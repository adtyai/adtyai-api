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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef, useState } from 'react'
import { toast, Toaster } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  PromptInput,
  PromptInputAttachments,
  PromptInputProvider,
  PromptInputSpeechButton,
  PromptInputSubmit,
  PromptInputTextarea,
  type PromptInputProps,
} from '../prompt-input'

class BrowserRecognition extends EventTarget {
  static instances: BrowserRecognition[] = []
  continuous = false
  interimResults = false
  lang = ''
  onstart: ((event: Event) => void) | null = null
  onend: ((event: Event) => void) | null = null
  onresult: ((event: Event) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  start = vi.fn()
  stop = vi.fn()
  abort = vi.fn()

  constructor() {
    super()
    BrowserRecognition.instances.push(this)
  }

  emit(type: 'start' | 'end' | 'result' | 'error', data = {}) {
    const event = Object.assign(new Event(type), data)
    this.dispatchEvent(event)
    this[`on${type}`]?.(event)
  }

  result(resultIndex: number, entries: [string, boolean][]) {
    const results = entries.map(([transcript, isFinal]) => {
      const alternative = { transcript, confidence: 1 }
      return {
        0: alternative,
        length: 1,
        isFinal,
        item: () => alternative,
      }
    })
    this.emit('result', {
      resultIndex,
      results: {
        ...results,
        length: results.length,
        item: (index: number) => results[index],
      },
    })
  }
}

function SpeechFixture(props: {
  onTranscriptionChange?: (text: string) => void
  showSpeech?: boolean
}) {
  const [text, setText] = useState('Draft')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  return (
    <>
      <textarea
        aria-label='Message'
        ref={textareaRef}
        value={text}
        onChange={(event) => setText(event.currentTarget.value)}
      />
      <output aria-label='Stored draft'>{text}</output>
      {props.showSpeech !== false && (
        <PromptInputSpeechButton
          aria-label='Dictate'
          textareaRef={textareaRef}
          onTranscriptionChange={props.onTranscriptionChange}
        />
      )}
      <Toaster />
    </>
  )
}

function SubmitFixture(props: {
  provider?: boolean
  onSubmit: PromptInputProps['onSubmit']
  accept?: string
  onError?: PromptInputProps['onError']
}) {
  const input = (
    <PromptInput
      accept={props.accept}
      multiple
      onError={props.onError}
      onSubmit={props.onSubmit}
    >
      <PromptInputTextarea aria-label='Message' />
      <PromptInputAttachments>
        {(file) => <span>{file.filename}</span>}
      </PromptInputAttachments>
      <PromptInputSubmit />
    </PromptInput>
  )
  return (
    <>
      {props.provider ? (
        <PromptInputProvider>{input}</PromptInputProvider>
      ) : (
        input
      )}
      <Toaster />
    </>
  )
}

function attachmentResponse() {
  const response = new Response()
  vi.spyOn(response, 'blob').mockResolvedValue(
    new Blob(['hello'], { type: 'text/plain' })
  )
  return response
}

beforeEach(() => {
  BrowserRecognition.instances = []
  vi.stubGlobal('SpeechRecognition', BrowserRecognition)
  vi.stubGlobal('webkitSpeechRecognition', undefined)
  let objectUrl = 0
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL = vi.fn(() => `blob:attachment-${++objectUrl}`)
      static revokeObjectURL = vi.fn()
    }
  )
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => attachmentResponse())
  )
})

afterEach(() => {
  toast.dismiss()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('PromptInput submission', () => {
  it.each([false, true])(
    'waits for successful submission before clearing attachments (provider=%s)',
    async (provider) => {
      const user = userEvent.setup()
      let finish!: () => void
      const pending = new Promise<void>((resolve) => {
        finish = resolve
      })
      const onSubmit = vi.fn(() => pending)
      render(<SubmitFixture provider={provider} onSubmit={onSubmit} />)
      await user.type(screen.getByRole('textbox', { name: 'Message' }), 'Hello')
      await user.upload(screen.getByLabelText('Upload files'), [
        new File(['first'], 'first.txt', { type: 'text/plain' }),
        new File(['second'], 'second.txt', { type: 'text/plain' }),
      ])
      await user.click(screen.getByRole('button', { name: 'Submit' }))
      await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce())
      expect(onSubmit).toHaveBeenCalledWith(
        {
          text: 'Hello',
          files: [
            {
              type: 'file',
              filename: 'first.txt',
              mediaType: 'text/plain',
              url: 'data:text/plain;base64,aGVsbG8=',
            },
            {
              type: 'file',
              filename: 'second.txt',
              mediaType: 'text/plain',
              url: 'data:text/plain;base64,aGVsbG8=',
            },
          ],
        },
        expect.anything()
      )
      expect(screen.getByText('first.txt')).toBeInTheDocument()
      if (provider) {
        expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
          'Hello'
        )
      }
      await act(async () => finish())
      await waitFor(() =>
        expect(screen.queryByText('first.txt')).not.toBeInTheDocument()
      )
      expect(screen.queryByText('second.txt')).not.toBeInTheDocument()
      expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue('')
    }
  )

  it('keeps existing attachment URLs alive while adding another file', async () => {
    const user = userEvent.setup()
    render(<SubmitFixture provider={false} onSubmit={vi.fn()} />)
    const input = screen.getByLabelText('Upload files')
    const revokeObjectURL = URL.revokeObjectURL as unknown as ReturnType<
      typeof vi.fn
    >

    await user.upload(input, new File(['first'], 'first.txt'))
    await user.upload(input, new File(['second'], 'second.txt'))

    expect(revokeObjectURL).not.toHaveBeenCalled()
    expect(screen.getByText('first.txt')).toBeInTheDocument()
    expect(screen.getByText('second.txt')).toBeInTheDocument()
  })

  it('handles file drops on the component form by default', () => {
    const { container } = render(
      <SubmitFixture provider={false} onSubmit={vi.fn()} />
    )
    const file = new File(['dropped'], 'dropped.txt', { type: 'text/plain' })
    const form = container.querySelector('form')
    if (!form) throw new Error('PromptInput form was not rendered')

    fireEvent.drop(form, {
      dataTransfer: { files: [file], types: ['Files'] },
    })

    expect(screen.getByText('dropped.txt')).toBeInTheDocument()
  })

  it('does not duplicate a form drop when global drop handling is enabled', () => {
    const { container } = render(
      <PromptInput globalDrop onSubmit={vi.fn()}>
        <PromptInputTextarea aria-label='Message' />
        <PromptInputAttachments>
          {(file) => <span>{file.filename}</span>}
        </PromptInputAttachments>
      </PromptInput>
    )
    const file = new File(['dropped'], 'global.txt', { type: 'text/plain' })
    const form = container.querySelector('form')
    if (!form) throw new Error('PromptInput form was not rendered')

    fireEvent.drop(form, {
      dataTransfer: { files: [file], types: ['Files'] },
    })

    expect(screen.getAllByText('global.txt')).toHaveLength(1)
  })

  it('revokes provider attachment URLs when the provider unmounts', async () => {
    const user = userEvent.setup()
    const rendered = render(
      <SubmitFixture provider onSubmit={vi.fn()} />
    )
    const revokeObjectURL = URL.revokeObjectURL as unknown as ReturnType<
      typeof vi.fn
    >

    await user.upload(
      screen.getByLabelText('Upload files'),
      new File(['provider'], 'provider.txt')
    )
    expect(revokeObjectURL).not.toHaveBeenCalled()

    rendered.unmount()

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:attachment-1')
  })

  it('applies file constraints when attachments use the provider', async () => {
    const onError = vi.fn()
    render(
      <PromptInputProvider>
        <PromptInput accept='image/*' onError={onError} onSubmit={vi.fn()}>
          <PromptInputTextarea aria-label='Message' />
          <PromptInputAttachments>
            {(file) => <span>{file.filename}</span>}
          </PromptInputAttachments>
        </PromptInput>
      </PromptInputProvider>
    )

    const input = screen.getByLabelText('Upload files') as HTMLInputElement
    const file = new File(['text'], 'notes.txt', { type: 'text/plain' })
    Object.defineProperty(input, 'files', { configurable: true, value: [file] })
    fireEvent.change(input)

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'accept' })
    )
    expect(screen.queryByText('notes.txt')).not.toBeInTheDocument()
  })

  it.each(['synchronous', 'asynchronous'])(
    'reports %s submit failure and preserves provider draft and attachments for retry',
    async (mode) => {
      const user = userEvent.setup()
      const error = new Error('Submission failed')
      const onSubmit = vi.fn<PromptInputProps['onSubmit']>(() => {
        if (mode === 'asynchronous') return Promise.reject(error)
        throw error
      })
      render(<SubmitFixture provider onSubmit={onSubmit} />)
      await user.type(
        screen.getByRole('textbox', { name: 'Message' }),
        'Retry me'
      )
      await user.upload(
        screen.getByLabelText('Upload files'),
        new File(['file'], 'retry.txt')
      )
      await user.click(screen.getByRole('button', { name: 'Submit' }))
      expect(await screen.findByText('Submission failed')).toBeVisible()
      expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
        'Retry me'
      )
      expect(screen.getByText('retry.txt')).toBeInTheDocument()
      onSubmit.mockImplementation(async () => undefined)
      await user.click(screen.getByRole('button', { name: 'Submit' }))
      await waitFor(() =>
        expect(screen.queryByText('retry.txt')).not.toBeInTheDocument()
      )
    }
  )

  it('reports attachment conversion failure before dispatch and permits retry', async () => {
    const user = userEvent.setup()
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error('Attachment unavailable'))
      .mockResolvedValue(attachmentResponse())
    vi.stubGlobal('fetch', fetch)
    const onSubmit = vi.fn()
    render(<SubmitFixture provider onSubmit={onSubmit} />)
    await user.type(
      screen.getByRole('textbox', { name: 'Message' }),
      'Keep this'
    )
    await user.upload(
      screen.getByLabelText('Upload files'),
      new File(['file'], 'retry.txt')
    )
    await user.click(screen.getByRole('button', { name: 'Submit' }))
    expect(await screen.findByText('Attachment unavailable')).toBeVisible()
    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getByText('retry.txt')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Keep this'
    )
    await user.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() =>
      expect(screen.queryByText('retry.txt')).not.toBeInTheDocument()
    )
    expect(onSubmit).toHaveBeenCalledOnce()
  })

  it('preserves the draft without an error toast when submission is cancelled', async () => {
    const user = userEvent.setup()
    let cancel!: (reason: unknown) => void
    const pending = new Promise<void>((_resolve, reject) => {
      cancel = reject
    })
    const onSubmit = vi.fn(() => pending)
    render(<SubmitFixture provider onSubmit={onSubmit} />)
    await user.type(
      screen.getByRole('textbox', { name: 'Message' }),
      'Keep this'
    )
    await user.upload(
      screen.getByLabelText('Upload files'),
      new File(['file'], 'retry.txt')
    )
    await user.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce())
    await act(async () =>
      cancel(new DOMException('Request cancelled', 'AbortError'))
    )
    expect(screen.queryByText('Request cancelled')).not.toBeInTheDocument()
    expect(screen.getByText('retry.txt')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Keep this'
    )
  })

  it('reports FileReader failure without dispatching or clearing the draft', async () => {
    class FailingReader extends FileReader {
      readAsDataURL() {
        Object.defineProperty(this, 'error', {
          value: new DOMException('Attachment read failed', 'NotReadableError'),
        })
        this.dispatchEvent(new Event('error'))
        this.dispatchEvent(new Event('loadend'))
      }
    }
    vi.stubGlobal('FileReader', FailingReader)
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<SubmitFixture provider onSubmit={onSubmit} />)
    await user.upload(
      screen.getByLabelText('Upload files'),
      new File(['file'], 'unreadable.txt')
    )
    await user.click(screen.getByRole('button', { name: 'Submit' }))
    expect(await screen.findByText('Attachment read failed')).toBeVisible()
    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getByText('unreadable.txt')).toBeInTheDocument()
  })
})

describe('PromptInput speech lifecycle', () => {
  it('disables dictation when neither browser speech constructor is available', () => {
    vi.stubGlobal('SpeechRecognition', undefined)
    render(<SpeechFixture />)
    expect(screen.getByRole('button', { name: 'Dictate' })).toBeDisabled()
  })

  it('supports the webkit constructor and follows browser start/end state', async () => {
    vi.stubGlobal('SpeechRecognition', undefined)
    vi.stubGlobal('webkitSpeechRecognition', BrowserRecognition)
    const user = userEvent.setup()
    render(<SpeechFixture />)
    const button = screen.getByRole('button', { name: 'Dictate' })
    const recognition = BrowserRecognition.instances[0]
    await user.click(button)
    expect(recognition.start).toHaveBeenCalledOnce()
    act(() => recognition.emit('start'))
    expect(button).toHaveAttribute('aria-pressed', 'true')
    await user.click(button)
    expect(recognition.stop).toHaveBeenCalledOnce()
    act(() => recognition.result(0, [['final words', true]]))
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Draft final words'
    )
    act(() => recognition.emit('end'))
    expect(button).toHaveAttribute('aria-pressed', 'false')
  })

  it('appends only changed final results and ignores interim results', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<SpeechFixture onTranscriptionChange={onChange} />)
    const recognition = BrowserRecognition.instances[0]
    await user.click(screen.getByRole('button', { name: 'Dictate' }))
    act(() => recognition.emit('start'))
    act(() => recognition.result(0, [['thinking', false]]))
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Draft'
    )
    act(() => recognition.result(0, [['Hello', true]]))
    act(() =>
      recognition.result(1, [
        ['Hello', true],
        ['world', true],
      ])
    )
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Draft Hello world'
    )
    expect(onChange).toHaveBeenLastCalledWith('Draft Hello world')
  })

  it('updates a controlled textarea through its native input event', async () => {
    const user = userEvent.setup()
    render(<SpeechFixture />)
    const recognition = BrowserRecognition.instances[0]
    await user.click(screen.getByRole('button', { name: 'Dictate' }))
    act(() => recognition.emit('start'))
    act(() => recognition.result(0, [['Hello', true]]))
    expect(screen.getByLabelText('Stored draft')).toHaveTextContent(
      'Draft Hello'
    )
  })

  it('keeps the active session while transcription callbacks change', async () => {
    const user = userEvent.setup()
    const oldCallback = vi.fn()
    const currentCallback = vi.fn()
    const { rerender } = render(
      <SpeechFixture onTranscriptionChange={oldCallback} />
    )
    const recognition = BrowserRecognition.instances[0]
    await user.click(screen.getByRole('button', { name: 'Dictate' }))
    act(() => recognition.emit('start'))
    rerender(<SpeechFixture onTranscriptionChange={currentCallback} />)
    expect(recognition.stop).not.toHaveBeenCalled()
    expect(recognition.abort).not.toHaveBeenCalled()
    act(() => recognition.result(0, [['Hello', true]]))
    expect(currentCallback).toHaveBeenCalledWith('Draft Hello')
    expect(oldCallback).not.toHaveBeenCalled()
  })

  it('stops a pending start instead of issuing a second start', async () => {
    const user = userEvent.setup()
    render(<SpeechFixture />)
    const recognition = BrowserRecognition.instances[0]
    const button = screen.getByRole('button', { name: 'Dictate' })
    await user.click(button)
    await user.click(button)
    expect(recognition.start).toHaveBeenCalledOnce()
    expect(recognition.stop).toHaveBeenCalledOnce()
    act(() => recognition.emit('end'))
    await user.click(button)
    expect(recognition.start).toHaveBeenCalledTimes(2)
  })

  it('reports a synchronous start failure and permits a later start', async () => {
    const user = userEvent.setup()
    render(<SpeechFixture />)
    const recognition = BrowserRecognition.instances[0]
    recognition.start.mockImplementationOnce(() => {
      throw new Error('Microphone busy')
    })
    const escaped: ErrorEvent[] = []
    const captureError = (event: ErrorEvent) => {
      escaped.push(event)
      event.preventDefault()
    }
    window.addEventListener('error', captureError)
    try {
      await user.click(screen.getByRole('button', { name: 'Dictate' }))
      expect(escaped).toHaveLength(0)
      expect(await screen.findByText('Microphone busy')).toBeVisible()
      await user.click(screen.getByRole('button', { name: 'Dictate' }))
      expect(recognition.start).toHaveBeenCalledTimes(2)
    } finally {
      window.removeEventListener('error', captureError)
    }
  })

  it('ignores results after speech errors and can start a later session', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<SpeechFixture onTranscriptionChange={onChange} />)
    const recognition = BrowserRecognition.instances[0]
    const button = screen.getByRole('button', { name: 'Dictate' })
    await user.click(button)
    act(() => recognition.emit('start'))
    act(() => recognition.emit('error', { error: 'network' }))
    act(() => recognition.result(0, [['stale', true]]))
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Draft'
    )
    act(() => recognition.emit('end'))
    await user.click(button)
    act(() => recognition.emit('start'))
    act(() => recognition.result(0, [['Hello', true]]))
    expect(onChange).toHaveBeenCalledWith('Draft Hello')
  })

  it('aborts and detaches speech results when the speech button unmounts', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const { rerender } = render(
      <SpeechFixture onTranscriptionChange={onChange} />
    )
    const recognition = BrowserRecognition.instances[0]
    await user.click(screen.getByRole('button', { name: 'Dictate' }))
    act(() => recognition.emit('start'))
    rerender(
      <SpeechFixture showSpeech={false} onTranscriptionChange={onChange} />
    )
    expect(recognition.abort).toHaveBeenCalledOnce()
    act(() => recognition.result(0, [['stale', true]]))
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue(
      'Draft'
    )
  })
})
