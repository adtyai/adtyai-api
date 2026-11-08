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
import type { AsyncRouteComponent } from '@tanstack/react-router'
import { lazy, type ComponentType } from 'react'

export function lazyRouteComponent<
  TModule extends Record<string, unknown>,
  TExport extends keyof TModule = 'default',
>(importer: () => Promise<TModule>, exportName?: TExport) {
  let importPromise: Promise<TModule> | undefined
  const load = () => (importPromise ??= Promise.resolve().then(importer))
  const component = lazy(async () => ({
    default: (await load())[exportName ?? 'default'] as ComponentType<unknown>,
  }))

  return Object.assign(component, {
    // Intent preloads leave the current screen alone. A render observes the
    // original rejection and delegates recovery to the shared error boundary.
    preload: () =>
      load().then(
        () => undefined,
        () => undefined
      ),
  }) as unknown as TModule[TExport] extends ComponentType<infer TProps>
    ? AsyncRouteComponent<TProps>
    : never
}
