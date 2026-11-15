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
/** The kicker, two-line heading and one-sentence lead each section opens with. */
export function SectionIntro(props: {
  headingId: string
  eyebrow: string
  title: string
  mutedTitle: string
  lead: string
}) {
  return (
    <>
      <p className='text-neutral font-mono text-[11.5px] tracking-[0.18em] uppercase'>
        {props.eyebrow}
      </p>
      <h2
        id={props.headingId}
        className='mt-6 text-[clamp(2rem,4.4vw,3.6rem)] leading-[1.08] font-extrabold tracking-[-0.04em] [text-wrap:balance] [word-break:keep-all]'
      >
        <span className='block'>{props.title}</span>
        <span className='text-neutral block'>{props.mutedTitle}</span>
      </h2>
      <p className='text-muted-foreground mt-6 max-w-[36rem] text-[clamp(0.95rem,1.3vw,1.125rem)] leading-[1.75] text-pretty'>
        {props.lead}
      </p>
    </>
  )
}
