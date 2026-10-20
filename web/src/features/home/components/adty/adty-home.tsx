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
import {
  Claude,
  DeepSeek,
  Doubao,
  Gemini,
  Grok,
  Kimi,
  Meta,
  Minimax,
  Mistral,
  OpenAI,
  Qwen,
  Zhipu,
} from '@lobehub/icons'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BarChart3,
  Plug,
  ShieldCheck,
  Users,
  Wallet,
  Zap,
} from 'lucide-react'
import type { ComponentType } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'


type IconComp = ComponentType<{ size?: number }>

const VENDORS: { name: string; Icon: IconComp }[] = [
  { name: 'OpenAI', Icon: OpenAI },
  { name: 'Claude', Icon: Claude.Color },
  { name: 'Gemini', Icon: Gemini.Color },
  { name: 'DeepSeek', Icon: DeepSeek.Color },
  { name: 'Qwen', Icon: Qwen.Color },
  { name: 'Grok', Icon: Grok },
  { name: 'Kimi', Icon: Kimi },
  { name: 'GLM', Icon: Zhipu.Color },
  { name: 'Doubao', Icon: Doubao.Color },
  { name: 'Mistral', Icon: Mistral.Color },
  { name: 'Llama', Icon: Meta.Color },
  { name: 'MiniMax', Icon: Minimax.Color },
]

function useSiteOrigin(): string {
  const { status } = useStatus()
  const addr =
    typeof status?.server_address === 'string' ? status.server_address : ''
  const origin =
    addr || (typeof window !== 'undefined' ? window.location.origin : '')
  return origin.replace(/\/+$/, '')
}

function SectionTitle(props: { title: string; sub?: string }) {
  return (
    <div className='mx-auto max-w-2xl text-center'>
      <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
        {props.title}
      </h2>
      {props.sub && (
        <p className='text-muted-foreground mt-3 text-sm md:text-base'>
          {props.sub}
        </p>
      )}
    </div>
  )
}

export function AdtyHero(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const origin = useSiteOrigin()
  const primaryTo = props.isAuthenticated ? '/keys' : '/sign-up'

  return (
    <section className='relative overflow-hidden'>
      <div className='pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_top,var(--color-indigo-50),transparent_60%)] dark:opacity-10' />
      <div className='relative mx-auto max-w-6xl px-6 pt-28 pb-16 text-center md:pt-36'>
        <span className='border-primary/20 bg-primary/5 text-primary inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium'>
          <Wallet className='size-3.5' />
          {t('Pay as you go')}
        </span>
        <h1 className='mt-6 text-4xl font-bold tracking-tight md:text-6xl'>
          {t('One key to access')}
          <span className='text-primary'>{t('all mainstream models')}</span>
        </h1>
        <p className='text-muted-foreground mx-auto mt-5 max-w-2xl text-base md:text-lg'>
          {t(
            'OpenAI-compatible API. Call GPT, Claude, Gemini, DeepSeek, Qwen and more in one place, with pay-as-you-go USD billing and a single bill.'
          )}
        </p>
        <div className='mt-8 flex flex-col justify-center gap-3 sm:flex-row'>
          <Button
            className='shadow-primary/20 h-11 rounded-xl px-6 text-sm font-medium shadow-lg'
            render={<Link to={primaryTo} />}
          >
            {t('Get API key')}
            <ArrowRight className='size-4' />
          </Button>
          <Button
            variant='outline'
            className='h-11 rounded-xl px-6 text-sm font-medium'
            render={<Link to='/pricing' />}
          >
            {t('View model pricing')}
          </Button>
        </div>
        <div className='mx-auto mt-12 max-w-2xl overflow-x-auto rounded-2xl border border-slate-800 bg-slate-950 p-5 text-left font-mono text-xs leading-6 text-slate-200 shadow-xl md:text-sm'>
          <div className='text-slate-500'>
            # {t('Just replace base_url and the key')}
          </div>
          <div className='whitespace-pre'>
            {`curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer sk-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'`}
          </div>
        </div>
        <div className='text-muted-foreground mx-auto mt-10 flex max-w-xl flex-wrap justify-center gap-x-6 gap-y-2 text-sm'>
          <span>{t('OpenAI-compatible')}</span>
          <span>·</span>
          <span>{t('Pay as you go')}</span>
          <span>·</span>
          <span>{t('Transparent multipliers')}</span>
        </div>
      </div>
    </section>
  )
}

export function AdtyVendorWall() {
  const { t } = useTranslation()
  return (
    <section className='bg-muted/40 border-border/60 border-y py-12'>
      <div className='mx-auto max-w-6xl px-6'>
        <p className='text-muted-foreground mb-8 text-center text-sm'>
          {t('Supported model vendors')}
        </p>
        <div className='grid grid-cols-3 gap-3 md:grid-cols-6 md:gap-4'>
          {VENDORS.map(({ name, Icon }) => (
            <div
              key={name}
              className='bg-background border-border text-foreground/80 flex h-14 items-center justify-center gap-2 rounded-xl border text-sm font-semibold'
            >
              <Icon size={20} />
              <span className='hidden sm:inline'>{name}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

export function AdtySteps() {
  const { t } = useTranslation()
  const steps = [
    {
      title: t('Create an account'),
      desc: t('Sign up with your email in a minute'),
    },
    {
      title: t('Top up and create a key'),
      desc: t('USD pricing · full amount credited · tier bonus extra'),
    },
    {
      title: t('Replace the API address'),
      desc: t('Point base_url to this site and start calling'),
    },
  ]
  return (
    <section className='mx-auto max-w-6xl px-6 py-20'>
      <SectionTitle
        title={t('Get started in three steps')}
        sub={t('Integrate in minutes without changing your business code')}
      />
      <div className='mt-12 grid gap-6 md:grid-cols-3'>
        {steps.map((s, i) => (
          <div key={s.title} className='border-border rounded-2xl border p-6'>
            <div className='bg-primary/10 text-primary grid size-10 place-items-center rounded-full font-bold'>
              {i + 1}
            </div>
            <h3 className='mt-4 text-lg font-semibold'>{s.title}</h3>
            <p className='text-muted-foreground mt-2 text-sm'>{s.desc}</p>
          </div>
        ))}
      </div>
    </section>
  )
}

export function AdtyFeatures() {
  const { t } = useTranslation()
  const items = [
    {
      Icon: Zap,
      title: t('Fast and stable'),
      desc: t('Multi-route scheduling with automatic failover'),
    },
    {
      Icon: Wallet,
      title: t('Transparent pricing'),
      desc: t('Pay as you go; model multipliers are public'),
    },
    {
      Icon: Plug,
      title: t('Fully compatible'),
      desc: t('OpenAI format; popular clients work out of the box'),
    },
    {
      Icon: BarChart3,
      title: t('Visible usage'),
      desc: t('Real-time logs and usage trends with clear bills'),
    },
    {
      Icon: ShieldCheck,
      title: t('Secure and controllable'),
      desc: t('Per-key quota, model and IP restrictions'),
    },
    {
      Icon: Users,
      title: t('Invite rewards'),
      desc: t('Invite friends and earn commission on their top-ups'),
    },
  ]
  return (
    <section className='bg-muted/40 border-border/60 border-y py-20'>
      <div className='mx-auto max-w-6xl px-6'>
        <SectionTitle title={t('Why choose us')} />
        <div className='mt-12 grid gap-6 sm:grid-cols-2 md:grid-cols-3'>
          {items.map(({ Icon, title, desc }) => (
            <div
              key={title}
              className='bg-background border-border rounded-2xl border p-6 transition-shadow hover:shadow-md'
            >
              <div className='bg-primary/10 text-primary grid size-10 place-items-center rounded-xl'>
                <Icon className='size-5' />
              </div>
              <h3 className='mt-4 font-semibold'>{title}</h3>
              <p className='text-muted-foreground mt-2 text-sm'>{desc}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

export function AdtyBonusBanner(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  return (
    <section className='mx-auto max-w-6xl px-6 py-20'>
      <div className='bg-primary text-primary-foreground flex flex-col items-center justify-between gap-6 rounded-3xl p-10 md:flex-row md:p-14'>
        <div className='text-center md:text-left'>
          <h2 className='text-2xl font-bold md:text-3xl'>{t('Pay as you go')}</h2>
          <p className='mt-2 text-sm opacity-80 md:text-base'>
            {t('USD pricing · full amount credited · tier bonus extra')}
          </p>
        </div>
        <Button
          variant='secondary'
          className='text-primary h-11 rounded-xl bg-white px-6 font-semibold hover:bg-white/90'
          render={<Link to={props.isAuthenticated ? '/wallet' : '/sign-up'} />}
        >
          {t('Top up now')}
        </Button>
      </div>
    </section>
  )
}
