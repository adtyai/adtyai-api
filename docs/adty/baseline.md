# 上游基线

任务 0.3 的记录：上游 v1.0.0-rc.47 原样不改时，`scripts/adty/check.sh` 每一步在本地 WSL 上的结果。后面的任务只看新增的失败，这里记下的失败不算（约定 F）。

上游原样就过不了 `bun run lint` 和 `bun run copyright:check`。check.sh 不改上游文件，这两步改成和两份基线清单比较（见「基线清单」），所以在基线上 check.sh 的退出码是 0（见「在测试副本里的结果」）。

## 提交和环境

- 提交：`e835b1bfc43b565d625537830dbfb19328925134`（上游标签 `v1.0.0-rc.47`），工作区没有改动。
- 日期：2026-10-31。
- 系统：WSL2 上的 Debian 13（13.5），x86_64，16 核；时区 UTC+8，`LANG=en_US.UTF-8`。上游 CI 用 ubuntu-latest，时区 UTC。
- 网络：proxy.golang.org 连不上，Go 用 `GOPROXY=https://goproxy.cn,direct`；api.openai.com 这类境外接口也连不上；npm 仓库和 github.com 能连。

| 工具 | 版本 |
| --- | --- |
| git | 2.47.3 |
| go | go1.26.10 linux/amd64（`GOTOOLCHAIN=go1.26.10+auto`，`CGO_ENABLED=1`） |
| bun | 1.4.4 |
| node | v24.10.0 |
| npm | 11.6.1 |
| gh | 2.103.3 |
| rg | 15.2.0 |
| vitest | 5.0.4（bun.lock 锁定） |
| oxlint | 1.74.0（bun.lock 锁定） |

## 在哪里跑

- 从方案 v2.5 起，仓库在 Windows 上，check.sh 只在 WSL 里的测试副本（`~/adtyai/mirror/main`）里跑：命令写进任务脚本，经 wrun.sh 运行，它每次先把 Windows 仓库的工作区同步到副本。`node_modules` 和 `web/dist` 只在副本里，Windows 仓库里没有。
- check.sh 切到仓库根目录以后，先看这个目录的实际路径（经符号链接进来的也解析成真实位置），在 /mnt 下（Windows 的盘）就打印说明、以退出码 1 结束，什么也不做。这个检查在建 `web/dist` 占位、装依赖这些会写文件的步骤之前，所以在 WSL 里对着 Windows 仓库误跑 check.sh，也不会往里面写文件。
- 0.3 第一次记录时仓库还在 WSL 里，「怎么跑的」「每一步的结果」「失败的步骤」「测试」几节是那时直接在仓库里跑的结果；「基线清单」和「在测试副本里的结果」是修复轮加的，后者是在副本里重跑的结果。

## 怎么跑的

1. check.sh 一步失败就停。为了看到每一步的结果，先把它的每一步单独跑一遍，命令相同，`go test` 另加 `-count=1`，不读测试缓存。后端在 `bun install` 之前跑，和上游 CI 的后端作业一样没有 `web/node_modules`。
2. 再完整跑一遍 `bash scripts/adty/check.sh`。
3. 最后在 `web` 里跑 `bun run build`，接着再跑一遍 `bash scripts/adty/check.sh --backend` 和 `bun run test`。

## 每一步的结果

用时是这台机器上第一次跑的数字，和缓存有关。

| 步骤 | 结果 | 用时 |
| --- | --- | --- |
| 工具检查（`git go bun node npm gh rg` 都不在 /mnt/c 下） | 通过 | 不到 1 秒 |
| `web/dist/index.html` 占位 | 原来没有，建了空文件 | 不到 1 秒 |
| 根模块 `go vet ./...` | 通过 | 36 秒 |
| 根模块 `go build ./...` | 通过 | 2 秒 |
| 根模块 `go test`（83 个包去掉 main 包，共 82 个） | 通过 | 48 秒 |
| relaykit `go vet ./...`、`go build ./...`、`go test ./...` | 通过 | 各 1 秒左右 |
| tokenkit `go vet ./...`、`go build ./...`、`go test ./...` | 通过 | 各 1 秒左右 |
| `bun install --frozen-lockfile` | 通过，装了 1193 个包 | 7 秒 |
| `bun run typecheck` | 通过，1405 个文件没有类型错误 | 1 秒 |
| `bun run lint` | **失败**：155 个 error，66 个 warning | 2 秒 |
| `bun run test` | 通过：200 个测试文件，2427 个测试 | 120 秒 |
| `bun run copyright:check` | **失败**：24 个文件的文件头不合要求 | 1 秒 |
| `bun run build`（不在 check.sh 里） | 通过，产物 69 MB；已入库的文件（包括 `src/routeTree.gen.ts`）没有变化 | 8 秒 |

- 第一版 check.sh（lint、copyright:check 直接看上游命令的退出码）完整运行：退出码 1，用时 47 秒。后端各步全部通过；前端 install、typecheck 通过，停在 `bun run lint`，后面的 test 和 copyright:check 没跑到。修复轮把这两步改成和清单比较以后，退出码是 0，见「在测试副本里的结果」。
- `bun run build` 之后：`bash scripts/adty/check.sh --backend` 退出码 0；构建出来的 `web/dist/index.html` 跑前跑后 sha256 相同，占位逻辑没有覆盖它。第二次 `bun run test` 同样是 200 个文件、2427 个测试全部通过。
- 上游 CI 跑的步骤（三个模块的 vet、build、test，前端 `bun check -b` 和 `bun run test`）在这里全部通过，和 CI 一致。

## 失败的步骤

两处失败都和环境无关：oxlint 的版本由 bun.lock 锁定，文件头检查只看文件内容。上游 CI 的前端作业（`.github/workflows/ci.yml`）只跑 install、`bun check -b` 和 `bun run test`，不跑 lint 和 copyright:check，所以这些问题一直留在上游代码里。check.sh 不再直接看这两个命令的退出码，改成和基线清单比较，见「基线清单」。

### bun run lint

输出最后一行是 `Found 66 warnings and 155 errors.`，退出码 1。warning 不影响退出码。error 按规则分：

| 规则 | error 数 |
| --- | --- |
| typescript(no-import-type-side-effects) | 56 |
| react(no-array-index-key) | 23 |
| unicorn(prefer-spread) | 16 |
| eslint(no-nested-ternary) | 14 |
| unicorn(prefer-string-replace-all) | 13 |
| eslint(curly) | 10 |
| unicorn(prefer-at) | 6 |
| unicorn(no-useless-spread) | 6 |
| react(jsx-no-useless-fragment) | 3 |
| unicorn(prefer-optional-catch-binding) | 2 |
| react(iframe-missing-sandbox) | 2 |
| typescript(no-non-null-assertion) | 1 |
| react(button-has-type) | 1 |
| promise(catch-or-return) | 1 |
| oxc(const-comparisons) | 1 |

error 分布在 92 个文件里（路径相对 `web/`，前面是 error 数）：

```text
 1 scripts/sync-i18n.mjs
 1 src/assets/brand-icons/icon-discord.tsx
 1 src/assets/brand-icons/icon-docker.tsx
 1 src/assets/brand-icons/icon-facebook.tsx
 1 src/assets/brand-icons/icon-figma.tsx
 1 src/assets/brand-icons/icon-github.tsx
 1 src/assets/brand-icons/icon-gitlab.tsx
 1 src/assets/brand-icons/icon-gmail.tsx
 1 src/assets/brand-icons/icon-linuxdo.tsx
 1 src/assets/brand-icons/icon-medium.tsx
 1 src/assets/brand-icons/icon-notion.tsx
 1 src/assets/brand-icons/icon-skype.tsx
 1 src/assets/brand-icons/icon-slack.tsx
 1 src/assets/brand-icons/icon-stripe.tsx
 1 src/assets/brand-icons/icon-telegram.tsx
 1 src/assets/brand-icons/icon-trello.tsx
 1 src/assets/brand-icons/icon-wechat.tsx
 1 src/assets/brand-icons/icon-whatsapp.tsx
 1 src/assets/brand-icons/icon-zoom.tsx
 1 src/assets/clerk-full-logo.tsx
 1 src/assets/clerk-logo.tsx
 2 src/assets/custom/icon-dir.tsx
 1 src/assets/custom/icon-layout-compact.tsx
 1 src/assets/custom/icon-layout-default.tsx
 1 src/assets/custom/icon-layout-full.tsx
 1 src/assets/custom/icon-sidebar-floating.tsx
 1 src/assets/custom/icon-sidebar-inset.tsx
 1 src/assets/custom/icon-sidebar-sidebar.tsx
 1 src/assets/custom/icon-theme-dark.tsx
 1 src/assets/custom/icon-theme-light.tsx
 1 src/assets/custom/icon-theme-system.tsx
 1 src/assets/logo.tsx
 8 src/components/ai-elements/prompt-input.tsx
 2 src/components/ai-elements/web-preview.tsx
 1 src/components/confirm-dialog.tsx
 1 src/components/copy-button.tsx
 2 src/components/data-table/core/column-header.tsx
 4 src/components/data-table/toolbar/faceted-filter.tsx
 1 src/components/data-table/toolbar/view-options.tsx
 2 src/components/layout/components/footer.tsx
 1 src/components/layout/components/nav-link-item.tsx
 2 src/components/layout/components/public-navigation.tsx
 1 src/components/layout/components/top-nav.tsx
 1 src/components/layout/config/top-nav.config.ts
 1 src/components/layout/lib/url-utils.ts
 2 src/components/layout/types.ts
 1 src/components/long-text.tsx
 1 src/components/page-transition.tsx
 3 src/components/risk-acknowledgement-dialog.tsx
 1 src/components/tag-input.tsx
 2 src/features/auth/lib/validation.ts
 1 src/features/channels/components/data-table-bulk-actions.tsx
 5 src/features/channels/lib/model-mapping-validation.ts
 1 src/features/channels/lib/upstream-update-utils.ts
 4 src/features/dashboard/components/models/models-chart-preferences.tsx
 2 src/features/dashboard/components/models/models-filter-dialog.tsx
 1 src/features/dashboard/components/ui/panel-wrapper.tsx
 1 src/features/dashboard/lib/api-info.ts
 2 src/features/dashboard/lib/text.ts
 1 src/features/home/components/gateway-card.tsx
12 src/features/home/components/hero-terminal-demo.tsx
 2 src/features/home/components/scrolling-icons.tsx
 1 src/features/home/constants.ts
 2 src/features/keys/components/data-table-bulk-actions.tsx
 1 src/features/keys/constants.ts
 1 src/features/models/components/deployment-access-guard.tsx
 2 src/features/models/components/prefill-group-shared.ts
 1 src/features/models/lib/deployments-utils.ts
 6 src/features/pricing/components/model-details-api.tsx
 4 src/features/pricing/components/model-details-apps.tsx
 1 src/features/pricing/constants.ts
 1 src/features/pricing/lib/filters.ts
 6 src/features/pricing/lib/mock-stats.ts
 1 src/features/rankings/index.tsx
 4 src/features/rankings/lib/format.ts
 1 src/features/redemption-codes/components/redemptions-provider.tsx
 1 src/features/subscriptions/components/subscriptions-provider.tsx
 1 src/features/subscriptions/constants.ts
 1 src/features/system-settings/content/dashboard-section.tsx
 1 src/features/system-settings/general/pricing-section.tsx
 1 src/features/system-settings/integrations/utils.ts
 1 src/features/system-settings/models/utils.ts
 1 src/features/system-settings/utils/json-parser.ts
 1 src/features/usage-logs/components/task-logs-filter-bar.tsx
 3 src/features/users/components/data-table-bulk-actions.tsx
 1 src/features/users/lib/user-actions.ts
 1 src/features/users/lib/user-form.ts
 1 src/features/wallet/components/creem-products-section.tsx
 1 src/hooks/use-table-url-state.ts
 3 src/lib/http-status-code-rules.ts
 2 src/lib/utils.ts
 1 src/routes/_authenticated/chat/$chatId.tsx
```

### bun run copyright:check

输出 `copyright: checked 1412, added 13, updated 11, skipped generated 1, skipped third-party 1`，退出码 1。脚本（`web/scripts/add-copyright.mjs`）要求文件第一行就是 AGPL 文件头，文件头和代码之间不空行。列出的 24 个文件（路径相对 `web/`）：

文件头后面多了一个空行（11 个，脚本记为 updated）：

```text
src/components/ai-elements/response-fade.ts
src/features/auth/lib/oauth-callback-mode.ts
src/features/channels/constants.ts
src/features/channels/lib/channel-field-update.ts
src/features/channels/lib/header-passthrough.ts
src/features/channels/lib/model-categories.ts
src/features/channels/lib/model-mapping-rules.ts
src/features/task-plugins/lib/host-protocols.ts
src/lib/localized-text.ts
src/lib/model-provider.ts
src/lib/session-hint.ts
```

文件头写在 import 后面（5 个，记为 added）：

```text
src/features/channels/components/__tests__/responses-websocket-setting.test.tsx
src/features/security/components/__tests__/login-session-utils.test.ts
src/features/task-plugins/__tests__/plugin-icon-image.test.tsx
src/features/task-plugins/components/javascript-viewer.tsx
src/features/task-plugins/components/plugin-detail-sheet.tsx
```

没有文件头（8 个，记为 added）：

```text
src/features/system-settings/request-policies/__tests__/routing.test.tsx
src/features/system-settings/request-policies/api.ts
src/features/system-settings/request-policies/decision-record.tsx
src/features/system-settings/request-policies/policy-label.ts
src/features/system-settings/request-policies/related-policy-link.tsx
src/features/system-settings/request-policies/routing-form.ts
src/features/system-settings/request-policies/routing-section.tsx
src/features/system-settings/request-policies/use-save-policy.ts
```

## 基线清单

上游原样就有的 lint error 和不合要求的文件头，不改上游文件去修（约定 B）。check.sh 的这两步由 `scripts/adty/ratchet.mjs` 跑，和两份入库的清单比较：

| 清单 | 每一行 | 现在 |
| --- | --- | --- |
| `scripts/adty/baseline-lint.txt` | 文件路径、一个空格、这个文件的 lint error 数 | 89 个文件，共 152 个 error |
| `scripts/adty/baseline-copyright.txt` | copyright:check 列出的一个文件 | 24 个文件 |

路径相对 `web/`，按路径排序。清单里只有上游文件。建立时内容和上一节列的完全一致：修复轮用脚本核对过，每一项在 `e835b1bf` 里都有，用 `--update` 重新生成的清单和入库的逐字节相同。P1 删掉了其中 3 个有 lint error 的上游文件（`features/redemption-codes/components/redemptions-provider.tsx`、`features/subscriptions/components/subscriptions-provider.tsx`、`features/subscriptions/constants.ts`，各 1 个 error），`--update` 后去掉了这 3 行，所以和上一节的 92 个文件、155 个 error 差 3。上一节和下面「在测试副本里的结果」记的是 rc.47 原样时的数字，不改。

- `node scripts/adty/ratchet.mjs lint`：跑 `bun run lint --format=json`，只数 error，warning 不算。有 error 的文件不在清单里，或者 error 比清单记的多，就失败，并列出这些文件里的全部 error（清单只记个数，分不出哪个是新加的）；其余情况通过。
- `node scripts/adty/ratchet.mjs copyright`：跑 `bun run copyright:check`，它列出的文件里有不在清单里的，就失败；其余情况通过。检查认的是 QuantumNous 的 AGPL 文件头和 `support@quantumnous.com`，换行、空行和文件头后面的空行都不算失败；完全没有这段内容的新文件仍然失败。`bun run copyright` 继续把文件头整理成同一种格式。
- 清单里的文件问题变少、改好了或者删掉了，照样通过，只提示可以更新清单。
- 工具没跑起来、输出格式不对、退出码和输出对不上，都算失败，不会悄悄通过。
- 修复轮验证过：在副本的 `web/src` 里临时放一个新文件，里面有 1 个 lint error、没有 AGPL 文件头，两步都以退出码 1 失败并列出这个文件；删掉以后两步都通过。把清单里某个文件的 error 数改小，或者从版权清单里去掉一个文件，也都会失败。
- 我们自己的文件不进清单，有问题就改掉。新文件的 AGPL 文件头照别的文件手工加；也可以在副本里跑 `bun run copyright`，再用 adty-pull.sh 只把自己的文件拉回 Windows 仓库。它也会改清单里的 24 个上游文件，这些不要拉回，下次同步就还原了。
- 更新清单：合并上游以后（实施方案第七节第 4 步），在副本里跑 `node scripts/adty/ratchet.mjs --update`。它把两个工具都跑一遍，两边的结果都可信才重写两份清单。再用 adty-pull.sh 把 `scripts/adty/baseline-lint.txt`、`scripts/adty/baseline-copyright.txt` 拉回 Windows 仓库，用 `git diff scripts/adty/` 检查：新出现的、error 变多的只能是上游文件；我们的文件出现在里面，先改掉再重新生成。清单和合并一起提交。

## 测试

后端三个模块和前端都没有失败的测试，也没有只在这个环境（网络、时区、语言、机器）下失败的测试，所以没有跳过任何测试，也没有测试的跳过清单。

## 在测试副本里的结果

修复轮在 WSL 测试副本里完整跑了一遍 check.sh，回答实施方案第十节第 18 条的第三项：上游测试在副本里的结果和 0.3 记下的基线是否一致。

- 日期 2026-10-31，工具版本和上面的表相同。Go 代码和 `web/` 都和 `e835b1bf` 相同，只多了我们的脚本、文档和 CI 配置。副本里的 `web/dist` 是 `bun run build` 的产物，跑前跑后 sha256 相同。
- 命令：`GOFLAGS=-count=1 bash scripts/adty/check.sh`，不带参数。`GOFLAGS=-count=1` 只让 `go test` 不读测试缓存，每个 Go 测试都实际跑了一遍；`go vet`、`go build` 不认这个参数，不受影响。
- 结果：退出码 0，用时 183 秒，最后一步是 `==> 检查全部通过`。

| 步骤 | 结果 | 用时 |
| --- | --- | --- |
| 工具检查 | 通过 | 不到 1 秒 |
| 根模块 `go vet ./...`、`go build ./...` | 通过 | 9 秒、3 秒 |
| 根模块 `go test`（82 个包） | 通过：47 个包 ok，35 个包没有测试文件，没有失败，没有读缓存 | 45 秒 |
| relaykit、tokenkit 的 vet、build、test | 通过：relaykit 9 个包 ok、8 个没有测试文件，tokenkit 1 个包 ok | 共 3 秒 |
| `bun install --frozen-lockfile` | 通过，没有变化 | 不到 1 秒 |
| `bun run typecheck` | 通过，1405 个文件没有类型错误 | 1 秒 |
| lint（和清单比较） | 通过：检查 1343 个文件，92 个文件共 155 个 error、66 个 warning，和清单相同 | 1 秒 |
| `bun run test` | 通过：200 个测试文件，2427 个测试 | 121 秒 |
| copyright:check（和清单比较） | 通过：检查 1412 个文件，列出 24 个，和清单相同 | 不到 1 秒 |

和 0.3 的基线比：Go 测试全部通过，vitest 200 个文件、2427 个测试全部通过，lint 和 copyright:check 的问题一个不多、一个不少，typecheck 的文件数也相同。第三项的答案是一致。

## web/node_modules 里的 .go 文件

装完前端依赖后，`web/node_modules` 里没有 .go 文件，也没有 go.mod。`GOWORK=off go list -e ./...` 装依赖前后都是 83 个包，没有 `web/` 下的包，根模块的 `./...` 不会把前端依赖扫进去。根模块的 go.mod 没有 `ignore` 指令；以后哪个前端依赖带了 .go 文件，根模块的 `go vet ./...`、`go build ./...` 和测试都会把它当成包来编译。

## 对后面任务的影响

- 在基线上，check.sh 不带参数的退出码是 0，后端、前端两半的每一步都通过。后面的任务跑 check.sh，哪一步失败都是新问题，要解决。
- lint、copyright:check 只拦清单以外的问题：新文件不能有 lint error，必须带 AGPL 文件头；改了清单里的上游文件，它的 error 数不能变多。warning 不拦。
- Go 测试、typecheck 和前端测试没有清单，也没有跳过的测试，必须全过。
- check.sh 只在测试副本里跑；在 /mnt 下跑会直接退出。
