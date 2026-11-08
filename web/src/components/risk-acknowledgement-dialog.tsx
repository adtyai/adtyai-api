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
import { useEffect, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

type RiskText = {
  /** Unique within its collection and retained by the caller across updates. */
  id: string
  text: string
}

type RequiredTextPart = RiskText & {
  type: 'input' | 'static'
  placeholder?: string
}

type Acknowledgement<Value> = {
  text: string
  value: Value
}

type RiskAcknowledgementDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: React.ReactNode
  items?: RiskText[]
  checklist?: RiskText[]
  requiredText?: string
  requiredTextParts?: RequiredTextPart[]
  inputPrompt?: string
  inputPlaceholder?: string
  mismatchHint?: string
  confirmText?: string
  cancelText?: string
  destructive?: boolean
  isLoading?: boolean
  onConfirm: () => void
  className?: string
}

function getRequiredTextRows(text: string) {
  return Math.max(1, Math.ceil([...text].length / 42))
}

function retainCurrentAcknowledgements<Value>(
  previous: Map<string, Acknowledgement<Value>>,
  requirements: RiskText[]
) {
  const currentText = new Map(requirements.map((item) => [item.id, item.text]))
  const next = new Map(previous)
  for (const [id, acknowledgement] of previous) {
    if (currentText.get(id) !== acknowledgement.text) next.delete(id)
  }
  return next.size === previous.size ? previous : next
}

export function RiskAcknowledgementDialog({
  open,
  onOpenChange,
  title,
  description,
  items = [],
  checklist = [],
  requiredText = '',
  requiredTextParts = [],
  inputPrompt,
  inputPlaceholder,
  mismatchHint,
  confirmText,
  cancelText,
  destructive = true,
  isLoading = false,
  onConfirm,
  className,
}: RiskAcknowledgementDialogProps) {
  const { t } = useTranslation()
  const dialogId = useId()
  const [checkedItems, setCheckedItems] = useState<
    Map<string, Acknowledgement<boolean>>
  >(() => new Map())
  const [typedTextResponse, setTypedTextResponse] = useState<
    Acknowledgement<string>
  >({ text: requiredText, value: '' })
  const [typedTextParts, setTypedTextParts] = useState<
    Map<string, Acknowledgement<string>>
  >(() => new Map())

  const requiredInputParts = useMemo(
    () => requiredTextParts.filter((part) => part.type === 'input'),
    [requiredTextParts]
  )
  const hasSegmentedRequiredText = requiredInputParts.length > 0
  const requiredTextToDisplay = hasSegmentedRequiredText
    ? requiredTextParts.map((part) => part.text).join('')
    : requiredText
  const typedText =
    typedTextResponse.text === requiredText ? typedTextResponse.value : ''

  useEffect(() => {
    setCheckedItems((previous) =>
      retainCurrentAcknowledgements(previous, open ? checklist : [])
    )
    setTypedTextParts((previous) =>
      retainCurrentAcknowledgements(previous, open ? requiredInputParts : [])
    )
    setTypedTextResponse((previous) =>
      previous.text === requiredText &&
      ((open && !hasSegmentedRequiredText) || previous.value === '')
        ? previous
        : { text: requiredText, value: '' }
    )
  }, [
    open,
    checklist,
    requiredInputParts,
    requiredText,
    hasSegmentedRequiredText,
  ])

  const allChecked = useMemo(() => {
    return checklist.every((item) => {
      const acknowledgement = checkedItems.get(item.id)
      return acknowledgement?.text === item.text && acknowledgement.value
    })
  }, [checkedItems, checklist])

  const typedMatched = useMemo(() => {
    if (hasSegmentedRequiredText) {
      return requiredInputParts.every((part) => {
        const acknowledgement = typedTextParts.get(part.id)
        return (
          acknowledgement?.text === part.text &&
          acknowledgement.value.trim() === part.text.trim()
        )
      })
    }
    if (!requiredText) return true
    return typedText.trim() === requiredText.trim()
  }, [
    hasSegmentedRequiredText,
    requiredInputParts,
    requiredText,
    typedText,
    typedTextParts,
  ])
  const hasTypedRequiredText = hasSegmentedRequiredText
    ? requiredInputParts.some((part) => {
        const acknowledgement = typedTextParts.get(part.id)
        return (
          acknowledgement?.text === part.text &&
          acknowledgement.value.trim() !== ''
        )
      })
    : typedText.length > 0

  const canConfirm = allChecked && typedMatched && !isLoading

  const handleChecklistChange = (item: RiskText, checked: boolean) => {
    setCheckedItems((previous) => {
      const next = new Map(previous)
      next.set(item.id, { text: item.text, value: checked })
      return next
    })
  }

  const handleTextPartChange = (part: RequiredTextPart, value: string) => {
    setTypedTextParts((previous) => {
      const next = new Map(previous)
      next.set(part.id, { text: part.text, value })
      return next
    })
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        className={cn(
          'flex max-h-[min(88dvh,760px)] w-[calc(100vw-1.5rem)] !max-w-[44rem] grid-rows-none flex-col gap-0 overflow-hidden !p-0 sm:w-[min(44rem,calc(100vw-3rem))]',
          className
        )}
      >
        <AlertDialogHeader className='shrink-0 px-4 pt-4 pb-3 text-left sm:px-6 sm:pt-6'>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description ? (
            <AlertDialogDescription
              render={<div />}
              className='mt-1 text-left leading-5'
            >
              {description}
            </AlertDialogDescription>
          ) : null}
        </AlertDialogHeader>

        <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-4 pb-4 sm:px-6'>
          {items.length > 0 ? (
            <ol className='border-border/70 bg-muted/30 text-foreground list-decimal space-y-2 rounded-lg border px-4 py-3 pl-8 text-sm leading-6 sm:px-5 sm:py-4 sm:pl-9'>
              {items.map((item) => (
                <li key={item.id}>{item.text}</li>
              ))}
            </ol>
          ) : null}

          {checklist.length > 0 ? (
            <div className='border-border/70 bg-muted/30 space-y-3 rounded-lg border p-3 sm:p-4'>
              {checklist.map((item) => {
                const id = `${dialogId}-risk-acknowledgement-${item.id}`
                const acknowledgement = checkedItems.get(item.id)
                return (
                  <div key={item.id} className='flex items-start gap-3'>
                    <Checkbox
                      id={id}
                      checked={
                        acknowledgement?.text === item.text &&
                        acknowledgement.value
                      }
                      onCheckedChange={(checked) =>
                        handleChecklistChange(item, checked === true)
                      }
                      className='mt-0.5'
                    />
                    <Label
                      htmlFor={id}
                      className='text-muted-foreground text-sm leading-5 font-normal'
                    >
                      {item.text}
                    </Label>
                  </div>
                )
              })}
            </div>
          ) : null}

          {requiredTextToDisplay ? (
            <div className='border-destructive/30 bg-destructive/5 space-y-3 rounded-lg border p-3 sm:p-4'>
              <Label className='text-sm font-medium'>
                {inputPrompt ?? t('Please type the following text to confirm:')}
              </Label>
              <div className='bg-background border-border rounded-md border px-3 py-2 font-mono text-sm leading-6 break-words whitespace-pre-wrap'>
                {requiredTextToDisplay}
              </div>
              {hasSegmentedRequiredText ? (
                <div className='flex flex-col gap-2'>
                  {requiredTextParts.map((part) =>
                    part.type === 'static' ? (
                      <span
                        key={part.id}
                        className='text-muted-foreground bg-background/70 border-border w-fit rounded-md border px-2 py-1.5 font-mono text-sm select-none'
                      >
                        {part.text}
                      </span>
                    ) : (
                      <Textarea
                        key={part.id}
                        value={
                          typedTextParts.get(part.id)?.text === part.text
                            ? (typedTextParts.get(part.id)?.value ?? '')
                            : ''
                        }
                        onChange={(event) =>
                          handleTextPartChange(part, event.target.value)
                        }
                        placeholder={
                          part.placeholder ??
                          part.text ??
                          inputPlaceholder ??
                          t('Type the confirmation text here')
                        }
                        rows={getRequiredTextRows(part.text)}
                        autoFocus={
                          open && part.id === requiredInputParts[0]?.id
                        }
                        wrap='soft'
                        aria-invalid={hasTypedRequiredText && !typedMatched}
                        className='min-h-10 resize-none overflow-hidden font-mono text-sm leading-6'
                      />
                    )
                  )}
                </div>
              ) : (
                <Textarea
                  value={typedText}
                  onChange={(event) =>
                    setTypedTextResponse({
                      text: requiredText,
                      value: event.target.value,
                    })
                  }
                  placeholder={
                    inputPlaceholder ?? t('Type the confirmation text here')
                  }
                  rows={getRequiredTextRows(requiredText)}
                  autoFocus={open}
                  wrap='soft'
                  aria-invalid={hasTypedRequiredText && !typedMatched}
                  className='min-h-16 resize-none overflow-hidden font-mono text-sm leading-6'
                />
              )}
              {hasTypedRequiredText && !typedMatched ? (
                <p className='text-destructive text-xs'>
                  {mismatchHint ??
                    t('The entered text does not match the required text.')}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>

        <AlertDialogFooter className='mx-0 mb-0 shrink-0 rounded-b-xl border-t p-3 sm:p-4'>
          <AlertDialogCancel disabled={isLoading}>
            {cancelText ?? t('Cancel')}
          </AlertDialogCancel>
          <Button
            variant={destructive ? 'destructive' : 'default'}
            disabled={!canConfirm}
            onClick={onConfirm}
          >
            {confirmText ?? t('Confirm')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
