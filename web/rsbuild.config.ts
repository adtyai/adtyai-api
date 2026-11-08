import path from 'node:path'
import { fileURLToPath } from 'node:url'

import type { ImportDeclaration } from '@babel/types'
import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'
import type { CodeSplitCompilerPlugin } from '@tanstack/router-plugin'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export const routeLazyRecoveryCompilerPlugin: CodeSplitCompilerPlugin = {
  name: 'route-lazy-recovery',
  onSplitRouteProperty(ctx) {
    if (ctx.splitNodeMeta.splitStrategy !== 'lazyRouteComponent') return
    for (const statement of ctx.programPath.get('body')) {
      if (
        !statement.isImportDeclaration() ||
        statement.node.source.value !== '@tanstack/react-router'
      ) {
        continue
      }

      const declaration = statement.node as ImportDeclaration
      const lazySpecifiers = declaration.specifiers.filter(
        (specifier) =>
          specifier.type === 'ImportSpecifier' &&
          specifier.local.name === ctx.lazyRouteComponentIdent &&
          specifier.imported.type === 'Identifier' &&
          specifier.imported.name === 'lazyRouteComponent'
      )
      if (lazySpecifiers.length === 0) continue

      const otherSpecifiers = declaration.specifiers.filter(
        (specifier) => !lazySpecifiers.includes(specifier)
      )
      ctx.programPath.unshiftContainer('body', {
        ...declaration,
        specifiers: lazySpecifiers,
        source: {
          ...declaration.source,
          value: '@/lib/route-lazy-component',
          extra: undefined,
        },
      })
      if (otherSpecifiers.length > 0) {
        declaration.specifiers = otherSpecifiers
      } else {
        statement.remove()
      }
    }
  },
}

export default defineConfig(({ envMode }) => {
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const serverUrl =
    process.env.VITE_REACT_APP_SERVER_URL ||
    env.rawPublicVars.VITE_REACT_APP_SERVER_URL ||
    'http://localhost:3000'

  const isProd = envMode === 'production'
  const devProxy = Object.fromEntries(
    (['/api', '/v1', '/mj', '/pg'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ])
  ) as Record<string, { target: string; changeOrigin: boolean }>

  return {
    plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
    // Rsbuild 2: replaces deprecated `performance.chunkSplit` (RSPack 2 aligned)
    splitChunks: {
      preset: 'default',
      cacheGroups: {
        'vendor-react': {
          test: /node_modules[\\/](react|react-dom)[\\/]/,
          name: 'vendor-react',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-ui-primitives': {
          test: /node_modules[\\/](@base-ui|@radix-ui)[\\/]/,
          name: 'vendor-ui-primitives',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-tanstack': {
          test: /node_modules[\\/]@tanstack[\\/]/,
          name: 'vendor-tanstack',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
      },
    },
    source: {
      entry: {
        index: './src/main.tsx',
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    html: {
      template: './index.html',
      favicon: './public/favicon.ico',
    },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      proxy: devProxy,
    },
    output: {
      // Production optimizations
      minify: isProd,
      target: 'web',
      distPath: {
        root: 'dist',
      },
      // Rely on Rsbuild default legalComments ("linked" → per-chunk *.LICENSE.txt) in all modes.
      // Do not set "none" in production: that strips minifier-preserved third-party notices and
      // extracted license files, which some distributions require for open-source compliance.
    },
    performance: {
      // Remove console in production
      removeConsole: isProd ? ['log'] : false,
      buildCache: false,
    },
    tools: {
      rspack: {
        // adty: Rsbuild only handles `?raw` for its own asset types; the docs
        // import their Markdown as strings (features/adty-docs/content).
        module: {
          rules: [
            { test: /\.md$/, resourceQuery: /raw/, type: 'asset/source' },
          ],
        },
        plugins: [
          tanstackRouter({
            target: 'react',
            // Dev: avoid per-route async chunks (reduces white flash on navigation + faster HMR feedback).
            // Prod: keep route-based code splitting.
            autoCodeSplitting: isProd,
            codeSplittingOptions: isProd
              ? { compilerPlugins: [routeLazyRecoveryCompilerPlugin] }
              : undefined,
          }),
        ],
      },
    },
  }
})
