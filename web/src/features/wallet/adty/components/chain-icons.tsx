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
type ChainIconProps = {
  method: string
  className?: string
}

// Simple marks for the four USDT chains. TRON follows the triangle mark of the
// previous fork; the others are plain circles with the chain's initial.
export function ChainIcon(props: ChainIconProps) {
  const className = props.className ?? 'size-6'
  if (props.method === 'usdt.trc20') {
    return (
      <svg viewBox='0 0 24 24' className={className} aria-hidden='true'>
        <path
          fill='#EF0027'
          d='M3.2 4.3 17.6 6l3.2 3.1-10 11.6L3.2 4.3Zm2.3 1.8 4.7 11.9 1-6.7-5.7-5.2Zm6.7 5.5-1 6.6 7.2-8.4-6.2 1.8Zm-5.5-5.9 5.1 4.6 4.2-1.2-9.3-3.4Zm9.8 1.6-.2.1 2.3 1.2-2.1-1.3Z'
        />
      </svg>
    )
  }
  const marks: Record<string, { color: string; letter: string }> = {
    'usdt.bep20': { color: '#F0B90B', letter: 'B' },
    'usdt.polygon': { color: '#8247E5', letter: 'P' },
    'usdt.xlayer': { color: '#0A0A0A', letter: 'X' },
  }
  const mark = marks[props.method] ?? { color: '#737373', letter: '?' }
  return (
    <svg viewBox='0 0 24 24' className={className} aria-hidden='true'>
      <circle cx='12' cy='12' r='11' fill={mark.color} />
      <text x='12' y='16' textAnchor='middle' fontSize='12' fontWeight='700' fill='#fff'>
        {mark.letter}
      </text>
    </svg>
  )
}
