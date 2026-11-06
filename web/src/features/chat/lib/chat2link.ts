export type Chat2LinkPreset = {
  type?: string
}

/** pending while status is still loading; missing when no web chat exists. */
export function chat2LinkOutcome(
  presets: Chat2LinkPreset[],
  loading: boolean
): 'pending' | 'missing' | 'ready' {
  if (loading) return 'pending'
  if (!presets.some((preset) => preset.type === 'web')) return 'missing'
  return 'ready'
}
