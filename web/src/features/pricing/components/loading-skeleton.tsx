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
import { Card, CardContent, CardFooter, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { VIEW_MODES, type ViewMode } from '../constants'

export interface LoadingSkeletonProps {
  viewMode?: ViewMode
}

const SKELETON_ROWS = ['first', 'second', 'third', 'fourth', 'fifth', 'sixth']

export function LoadingSkeleton(props: LoadingSkeletonProps) {
  return (
    <div aria-busy='true'>
      <div className='mb-8 max-w-2xl'>
        <Skeleton className='h-12 w-64 max-w-full' />
        <Skeleton className='mt-3 h-4 w-56 max-w-full' />
      </div>

      <div className='flex flex-wrap items-center gap-2 rounded-[14px] border p-2.5'>
        <Skeleton className='h-9 w-full sm:w-[280px]' />
        <Skeleton className='h-9 w-40' />
        <div className='ml-auto flex flex-wrap items-center gap-2'>
          <Skeleton className='h-9 w-24' />
          <Skeleton className='h-9 w-36' />
        </div>
      </div>

      {props.viewMode === VIEW_MODES.TABLE ? (
        <div className='mt-5 overflow-hidden rounded-[14px] border'>
          <div className='bg-muted/40 flex items-center gap-4 border-b px-3.5 py-3'>
            <Skeleton className='h-3 w-24' />
            <Skeleton className='h-3 w-16' />
            <Skeleton className='h-3 w-20' />
          </div>
          {SKELETON_ROWS.map((row) => (
            <div
              key={row}
              className='flex h-14 items-center gap-4 border-b px-3.5 last:border-b-0'
            >
              <Skeleton className='h-4 w-40 max-w-full' />
              <Skeleton className='h-4 w-20' />
              <Skeleton className='h-4 flex-1' />
              <Skeleton className='h-4 w-16' />
            </div>
          ))}
        </div>
      ) : (
        <div className='mt-5 grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 xl:grid-cols-3'>
          {SKELETON_ROWS.map((row) => (
            <Card key={row} className='gap-3'>
              <CardHeader className='flex flex-row gap-3'>
                <Skeleton className='size-10 shrink-0' />
                <div className='flex min-w-0 flex-1 flex-col gap-2'>
                  <Skeleton className='h-5 w-40 max-w-full' />
                  <Skeleton className='h-3 w-20' />
                </div>
                <Skeleton className='size-7 shrink-0' />
              </CardHeader>
              <CardContent className='flex flex-1 flex-col gap-3'>
                <div className='flex flex-col gap-2'>
                  <Skeleton className='h-3.5 w-full' />
                  <Skeleton className='h-3.5 w-4/5' />
                </div>
                <div className='mt-auto flex flex-col gap-1.5'>
                  <Skeleton className='h-4 w-16' />
                  <div className='grid grid-cols-3 gap-3'>
                    <Skeleton className='h-10' />
                    <Skeleton className='h-10' />
                    <Skeleton className='h-10' />
                  </div>
                </div>
              </CardContent>
              <CardFooter className='border-0 bg-transparent pt-0'>
                <Skeleton className='h-7 w-24' />
              </CardFooter>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
