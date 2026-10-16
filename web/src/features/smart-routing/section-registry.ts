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

export const SMART_ROUTING_SECTIONS = [
  {
    id: 'strategy',
    titleKey: 'Routing strategy',
  },
  {
    id: 'scores',
    titleKey: 'Channel scores & half-open',
  },
  {
    id: 'rankings',
    titleKey: 'Rankings by model',
  },
  {
    id: 'cost',
    titleKey: 'Cost guardrails',
  },
  {
    id: 'decisions',
    titleKey: 'Routing decisions',
  },
] as const

export type SmartRoutingSectionId =
  (typeof SMART_ROUTING_SECTIONS)[number]['id']

export const SMART_ROUTING_SECTION_IDS = SMART_ROUTING_SECTIONS.map(
  (s) => s.id
) as readonly SmartRoutingSectionId[]

export const SMART_ROUTING_DEFAULT_SECTION: SmartRoutingSectionId = 'strategy'

export function isSmartRoutingSectionId(s: string): s is SmartRoutingSectionId {
  return (SMART_ROUTING_SECTION_IDS as readonly string[]).includes(s)
}

export function getSmartRoutingSectionTitleKey(
  id: SmartRoutingSectionId
): string {
  const found = SMART_ROUTING_SECTIONS.find((s) => s.id === id)
  return found?.titleKey ?? 'Smart Routing'
}
