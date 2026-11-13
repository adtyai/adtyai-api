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
import { useNavigate, useRouterState } from '@tanstack/react-router'
import i18next from 'i18next'
import { useCallback } from 'react'

import { normalizeInterfaceLanguage } from '@/i18n/languages'

import {
  findHomeLangBySegment,
  HOME_LANG_STORAGE_KEY,
  homePathFor,
  isHomePath,
  officialHomeLangFor,
  type HomeLang,
} from './home-langs'

/** Official interface language picked on the home page, or null when never picked. */
export function readHomeLang(): string | null {
  try {
    const value = window.localStorage.getItem(HOME_LANG_STORAGE_KEY)
    if (!value) return null
    // Leftover unofficial values (zhTW, fr, …) resolve to an official language.
    return officialHomeLangFor(value).interfaceLang
  } catch {
    return null
  }
}

export function rememberHomeLang(interfaceLang: string): void {
  try {
    window.localStorage.setItem(
      HOME_LANG_STORAGE_KEY,
      officialHomeLangFor(interfaceLang).interfaceLang
    )
  } catch {
    // Private mode: the choice simply is not remembered.
  }
}

/**
 * Where `/` sends a visitor (adty task 9.3): the remembered home language,
 * else the interface language of a signed-in user, unless it is English.
 * Null keeps the visitor on `/`, which is English for search engines and
 * first-time visitors.
 */
export function resolveRootHomePath(options: {
  remembered: string | null
  signedIn: boolean
  interfaceLang: string
}): string | null {
  const lang =
    options.remembered ?? (options.signedIn ? options.interfaceLang : 'en')
  const path = homePathFor(normalizeInterfaceLanguage(lang))
  return path === '/' ? null : path
}

/** beforeLoad of `/`: the redirect pathname, or null to keep the root URL. */
export async function prepareRootHome(
  signedIn: boolean
): Promise<string | null> {
  const target = resolveRootHomePath({
    remembered: readHomeLang(),
    signedIn,
    interfaceLang: i18next.resolvedLanguage || i18next.language,
  })
  if (target) return target
  if (normalizeInterfaceLanguage(i18next.language) !== 'en') {
    await i18next.changeLanguage('en')
  }
  return null
}

export type PreparedLangHome = {
  lang: HomeLang
  /** Official path when the requested segment is a leftover unofficial home. */
  redirectTo?: string
}

/**
 * beforeLoad of `/$lang`: switches the interface to the official language for
 * the URL and remembers it. Leftover unofficial segments stay registered so
 * old links do not 404, but they resolve to the official language and should
 * redirect to the official home path. Undefined for an unknown segment.
 */
export async function prepareLangHome(
  segment: string
): Promise<PreparedLangHome | undefined> {
  const requested = findHomeLangBySegment(segment)
  if (!requested) return undefined
  const official = officialHomeLangFor(requested.interfaceLang)
  if (i18next.language !== official.interfaceLang) {
    await i18next.changeLanguage(official.interfaceLang)
  }
  rememberHomeLang(official.interfaceLang)
  if (requested.segment !== official.segment) {
    return { lang: official, redirectTo: homePathFor(official.interfaceLang) }
  }
  return { lang: official }
}

/**
 * Called by the language switcher after a switch: on a home page it moves
 * to the home URL of the new language and remembers it; elsewhere nothing.
 */
export function useAdtyAfterLanguageChange(): (code: string) => void {
  const navigate = useNavigate()
  const location = useRouterState({ select: (state) => state.location })
  return useCallback(
    (code: string) => {
      if (!isHomePath(location.pathname)) return
      rememberHomeLang(code)
      const target = homePathFor(code)
      const current = location.pathname.replace(/\/+$/, '') || '/'
      if (target === current) return
      void navigate({ href: target + location.searchStr })
    },
    [location.pathname, location.searchStr, navigate]
  )
}
