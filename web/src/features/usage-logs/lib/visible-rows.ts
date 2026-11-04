/** Rows that fit the viewport. Before measurement, use the reserved height. */
export function visibleRowRange(
  count: number,
  rowHeight: number,
  viewportHeight: number,
  reservedHeight = 0
): { start: number; end: number } {
  if (count <= 0) return { start: 0, end: 0 }
  const height = viewportHeight > 0 ? viewportHeight : reservedHeight
  const visible = height > 0 ? Math.ceil(height / rowHeight) : 1
  return { start: 0, end: Math.min(count, visible) }
}
