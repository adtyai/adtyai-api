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

function fileStem(name: string): string {
  const dot = name.lastIndexOf('.')
  const stem = dot > 0 ? name.slice(0, dot) : name
  return stem || 'icon'
}

function loadImageElement(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('image decode failed'))
    image.src = url
  })
}

/** Rasterise an image file into a square PNG with the artwork centred and
 *  letterboxed on a transparent background. Returns null when the browser
 *  cannot decode the file (e.g. jsdom). */
export async function toSquareIconFile(
  file: File,
  size = 256
): Promise<File | null> {
  let objectUrl: string | null = null
  try {
    if (typeof document === 'undefined') return null
    const canvas = document.createElement('canvas')
    canvas.width = size
    canvas.height = size
    // jsdom and other DOM shims cannot rasterise, so bail out before decoding.
    const context = canvas.getContext('2d')
    if (!context) return null

    let source: CanvasImageSource | null = null
    let width = 0
    let height = 0

    if (typeof createImageBitmap === 'function') {
      try {
        const bitmap = await createImageBitmap(file)
        source = bitmap
        width = bitmap.width
        height = bitmap.height
      } catch {
        source = null
      }
    }

    if (!source) {
      objectUrl = URL.createObjectURL(file)
      const image = await loadImageElement(objectUrl)
      source = image
      width = image.naturalWidth || image.width
      height = image.naturalHeight || image.height
    }

    if (!source || width <= 0 || height <= 0) return null

    const scale = Math.min(size / width, size / height)
    const drawWidth = width * scale
    const drawHeight = height * scale
    context.clearRect(0, 0, size, size)
    context.drawImage(
      source,
      (size - drawWidth) / 2,
      (size - drawHeight) / 2,
      drawWidth,
      drawHeight
    )

    const blob = await new Promise<Blob | null>((resolve) => {
      try {
        canvas.toBlob((result) => resolve(result), 'image/png')
      } catch {
        resolve(null)
      }
    })
    if (!blob) return null

    return new File([blob], `${fileStem(file.name)}.png`, { type: 'image/png' })
  } catch {
    return null
  } finally {
    if (objectUrl && typeof URL.revokeObjectURL === 'function') {
      URL.revokeObjectURL(objectUrl)
    }
  }
}
